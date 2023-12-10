import tempfile
from pathlib import Path

from slidge import global_config

global_config.HOME_DIR = Path(tempfile.gettempdir())
