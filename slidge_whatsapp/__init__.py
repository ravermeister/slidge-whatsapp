"""
WhatsApp gateway using the multi-device API.
"""

from slidge.util.util import get_version  # noqa: F401

from . import config, contact, group, session
from .gateway import Gateway

__version__ = get_version()
__all__ = "Gateway", "session", "contact", "config", "group"
