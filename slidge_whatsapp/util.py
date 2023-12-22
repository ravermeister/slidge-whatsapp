from os import fdopen
from tempfile import mkstemp
from typing import Optional

from slidge.core import config as global_config


async def get_bytes_temp(buf: bytes) -> Optional[str]:
    temp_file, temp_path = mkstemp(dir=global_config.HOME_DIR / "tmp")
    with fdopen(temp_file, "wb") as f:
        f.write(buf)
        return temp_path
