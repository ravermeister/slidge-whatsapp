# slidge-whatsapp

[Home](https://sr.ht/~nicoco/slidge) |
[Docs](https://slidge.im/slidge-whatsapp) |
[Issues](https://sr.ht/~nicoco/slidge/slidge-whatsapp) |
[Patches](https://lists.sr.ht/~nicoco/public-inbox) |
[Chat](xmpp:slidge@conference.nicoco.fr?join)

A
[feature-rich](https://slidge.im/slidge-whatsapp/features.html)
[WhatsApp](https://whatsapp.com) to
[XMPP](https://xmpp.org/) puppeteering
[gateway](https://xmpp.org/extensions/xep-0100.html), based on
[slidge](https://slidge.im) and
[whatsmeow](https://github.com/tulir/whatsmeow).

[![builds.sr.ht status](https://builds.sr.ht/~nicoco/slidge-whatsapp/commits/master/ci.yml.svg)](https://builds.sr.ht/~nicoco/slidge-whatsapp/commits/master/ci.yml)
[![containers status](https://builds.sr.ht/~nicoco/slidge-whatsapp/commits/master/container.yml.svg)](https://builds.sr.ht/~nicoco/slidge-whatsapp/commits/master/container.yml)
[![pypi status](https://badge.fury.io/py/slidge-whatsapp.svg)](https://pypi.org/project/slidge-whatsapp/)

## Installation

Refer to the [slidge admin documentation](https://slidge.im/core/admin/)
for general info on how to set up an XMPP server component.

### Containers

From [dockerhub](https://hub.docker.com/r/nicocool84/slidge-whatsapp)

```sh
docker run docker.io/nicocool84/slidge-whatsapp
```

### Python package

With [pipx](https://pypa.github.io/pipx/):

```sh
pipx install slidge-whatsapp  # for the latest tagged release
slidge-whatsapp --help
```

For the bleeding edge, download artifacts of
[this build job](https://builds.sr.ht/~nicoco/slidge-whatsapp/commits/master/ci.yml).

## Dev

```sh
git clone https://git.sr.ht/~nicoco/slidge
git clone https://git.sr.ht/~nicoco/slidge-whatsapp
cd slidge-whatsapp
docker-compose up
```
