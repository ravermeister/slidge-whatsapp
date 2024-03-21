ARG PYTHONVER=3.11
## Base build stage for Slidge, prepares and installs common dependencies.
FROM docker.io/library/python:$PYTHONVER-bookworm AS builder
ARG PYTHONVER
ENV PATH="/venv/bin:/root/.local/bin:$PATH"

# rust/cargo is for building "cryptography" since they don't provide wheels for arm32
RUN echo "deb http://deb.debian.org/debian bookworm-backports main" >> /etc/apt/sources.list \
    && apt-get update -y && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    cargo \
    curl \
    git \
    gcc \
    g++ \
    libffi-dev \
    libssl-dev \
    pkg-config \
    python3-dev \
    rustc \
    && apt-get install -y golang -t bookworm-backports

RUN pip install poetry
RUN python3 -m venv /venv
RUN ln -s /venv/lib/python$PYTHONVER /venv/lib/python

WORKDIR /build

ENV GOBIN="/usr/local/bin"
RUN go install -v github.com/go-python/gopy@latest
RUN go install golang.org/x/tools/cmd/goimports@latest

ENV PATH="/root/.local/bin:$PATH"
COPY poetry.lock pyproject.toml /build/

RUN poetry export --without-hashes > requirements.txt
RUN python3 -m pip install --requirement requirements.txt

COPY ./slidge_whatsapp/*.go ./slidge_whatsapp/go.* /build/
RUN gopy build -output=generated -no-make=true /build/

#FROM docker.io/nicocool84/slidge-base AS slidge-whatsapp
FROM docker.io/ravermeister/slidge-base AS slidge-whatsapp
USER root
RUN apt update -y && apt install ffmpeg -y

COPY --from=builder /venv /venv
COPY ./slidge_whatsapp/*.py /venv/lib/python/site-packages/legacy_module/
COPY --from=builder /build/generated /venv/lib/python/site-packages/legacy_module/generated

USER slidge

FROM builder AS slidge-whatsapp-dev

COPY --from=docker.io/nicocool84/slidge-prosody-dev:latest /etc/prosody/certs/localhost.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates

RUN apt update -y && apt install ffmpeg -y
RUN pip install watchdog[watchmedo]
ENV SLIDGE_LEGACY_MODULE=slidge_whatsapp

COPY ./watcher.py /
USER root

ENTRYPOINT ["python", "/watcher.py", "/venv/lib/python/site-packages/slidge:/venv/lib/python/site-packages/slidge_whatsapp"]

# wheel builder
# docker buildx build . --target wheel \
# --platform linux/arm64,linux/amd64 \
# -o ./dist/
FROM builder AS builder-wheel

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
