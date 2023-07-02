FROM docker.io/nicocool84/slidge-builder AS builder-base

RUN echo "deb http://deb.debian.org/debian bullseye-backports main" > /etc/apt/sources.list.d/backports.list && \
    apt update -y && \
    apt install -yt bullseye-backports golang

ENV GOBIN="/usr/local/bin"
RUN go install github.com/go-python/gopy@latest
RUN go install golang.org/x/tools/cmd/goimports@latest

ENV PATH="/root/.local/bin:$PATH"
COPY poetry.lock pyproject.toml /build/

RUN poetry export --without-hashes > requirements.txt
RUN python3 -m pip install --requirement requirements.txt

COPY ./slidge_whatsapp/*.go ./slidge_whatsapp/go.* /build/
RUN gopy build -output=generated -no-make=true /build/

FROM docker.io/nicocool84/slidge-base AS slidge-whatsapp

USER root
RUN apt update -y && apt install ffmpeg mediainfo -y

USER slidge
COPY --from=builder-base /venv /venv
COPY ./slidge_whatsapp/*.py /venv/lib/python/site-packages/legacy_module/
COPY --from=builder-base /build/generated /venv/lib/python/site-packages/legacy_module/generated

FROM builder-base AS slidge-whatsapp-dev

COPY --from=docker.io/nicocool84/slidge-prosody-dev:latest /etc/prosody/certs/localhost.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

RUN pip install watchdog[watchmedo]
ENV SLIDGE_LEGACY_MODULE=slidge_whatsapp

COPY ./watcher.py /
USER root
RUN apt update -y && apt install ffmpeg mediainfo -y

ENTRYPOINT ["python", "/watcher.py", "/venv/lib/python/site-packages/slidge:/venv/lib/python/site-packages/slidge_whatsapp"]

# wheel builder
# docker buildx build . --target wheel \
# --platform linux/arm64,linux/amd64 \
# -o ./dist/
FROM builder-base AS builder-wheel

RUN pip install pybindgen
COPY go.* /build/
COPY README.md /build/
COPY slidge_whatsapp/*.py /build/slidge_whatsapp/
COPY slidge_whatsapp/*.go /build/slidge_whatsapp/
COPY build.py /build/

RUN poetry build
RUN ls -l ./dist
RUN python --version

FROM scratch as wheel
COPY --from=builder-wheel ./build/dist/* /
