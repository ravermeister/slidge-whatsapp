"""
Config contains plugin-specific configuration for WhatsApp, and is loaded automatically by the
core configuration framework.
"""

from slidge import global_config

# FIXME: workaround because global_config.HOME_DIR is not defined unless
#        called by slidge's main(), which is a problem for test and docs
try:
    DB_PATH = global_config.HOME_DIR / "whatsapp" / "whatsapp.db"
    DB_PATH__DOC = "The path to the database used for the WhatsApp plugin."
except AttributeError:
    pass

ALWAYS_SYNC_ROSTER = False
ALWAYS_SYNC_ROSTER__DOC = (
    "Whether or not to perform a full sync of the WhatsApp roster on startup."
)

SKIP_VERIFY_TLS = False
SKIP_VERIFY_TLS__DOC = (
    "Whether or not HTTPS connections made by this plugin should verify TLS"
    " certificates."
)

ENABLE_LINK_PREVIEWS = True
ENABLE_LINK_PREVIEWS__DOC = (
    "Whether or not previews for links (URLs) should be generated on outgoing messages"
)