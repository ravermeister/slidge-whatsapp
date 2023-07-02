import io
import json
import logging
import os
import tempfile
from asyncio import AbstractEventLoop
from concurrent.futures import ThreadPoolExecutor
from os.path import basename
from subprocess import check_output

from aiohttp import ClientResponse
from PIL import Image

from . import config
from .generated import whatsapp


class ConversionError(BaseException):
    pass


class RGBAError(ConversionError):
    pass


class MediaConverter:
    pool: ThreadPoolExecutor

    def __init__(self, loop: AbstractEventLoop):
        if config.CONVERT_MEDIA:
            self.pool = ThreadPoolExecutor(config.CONVERT_MEDIA_THREADS)
        self.loop = loop

    async def _background(self, func, *args):
        return await self.loop.run_in_executor(self.pool, func, *args)

    async def convert(self, url: str, http_response: ClientResponse):
        filename = basename(str(http_response.url))

        if not config.CONVERT_MEDIA:
            return _return_url(url, filename, http_response)

        content_type = http_response.content_type
        type_, rest = content_type.lower().split("/")
        if type_ == "image" and "jpeg" not in rest and "jpg" not in rest:
            try:
                return whatsapp.Attachment(
                    MIME="image/jpeg",
                    Filename=filename,
                    Path=await self._background(
                        _convert_image, await http_response.read()
                    ),
                )
            except RGBAError:
                log.debug("Not converting RGBA image to JPEG")
            except Exception as e:
                log.error("Could not convert image", exc_info=e)
        elif type_ == "audio":
            try:
                return whatsapp.Attachment(
                    MIME="audio/ogg; codec=opus",
                    Filename=filename,
                    Path=await self._background(
                        _convert_audio, await http_response.read()
                    ),
                )
            except Exception as e:
                log.error("Could not convert audio", exc_info=e)

        # fallback in case conversion went wrong
        return _return_url(url, filename, http_response)


def _convert_image(data: bytes):
    image = Image.open(io.BytesIO(data))
    if image.mode == "RGBA":
        if config.CONVERT_RGBA:
            image = image.convert("RGB")
        else:
            raise RGBAError
    with tempfile.NamedTemporaryFile(suffix=".jpg", delete=False) as f:
        image.save(f.name, format="JPEG")
    return f.name


def _convert_audio(data: bytes):
    with tempfile.NamedTemporaryFile(delete=False) as f:
        f.write(data)
    container, codec = get_audio_format(f.name)
    if container == "Ogg" and codec == "Opus":
        return f.name

    with tempfile.NamedTemporaryFile(suffix=".oga", delete=False) as f2:
        check_output(["ffmpeg", "-i", f.name, "-acodec", "libopus", f2.name])
    os.unlink(f.name)
    return f2.name


def _return_url(url: str, filename: str, http_response: ClientResponse):
    return whatsapp.Attachment(
        MIME=http_response.content_type, Filename=filename, URL=url
    )


def get_audio_format(file_name: str):
    output = check_output(["mediainfo", file_name, "--Output=JSON"])
    data = json.loads(output.decode())
    tracks = data.get("media").get("track")
    if len(tracks) != 2:
        raise ConversionError("Tracks !?", tracks)
    container = tracks[0].get("Format")
    codec = tracks[1].get("Format")
    return container, codec


log = logging.getLogger(__name__)
