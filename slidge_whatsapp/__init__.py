"""
WhatsApp gateway using the multi-device API.
"""

from slidge import entrypoint
from slidge.util.util import get_version  # noqa: F401

from . import command, config, contact, group, session
from .gateway import Gateway


def main():
    entrypoint("slidge_whatsapp")


__version__ = get_version()
__all__ = "Gateway", "session", "command", "contact", "config", "group", "main"
