project = "slidge-whatsapp"
copyright = "2023, deuill"
author = "deuill"

extensions = [
    "sphinx.ext.autodoc",
    "sphinx.ext.intersphinx",
    "sphinx.ext.extlinks",
    "sphinx.ext.viewcode",
    "sphinx.ext.autodoc.typehints",
    "sphinxarg.ext",
    "autoapi.extension",
    "slidge_dev_helpers.doap",
    "slidge_dev_helpers.sphinx_config_obj",
    "sphinx_mdinclude",
]

autodoc_typehints = "description"

# Include __init__ docstrings
autoclass_content = "both"
autoapi_python_class_content = "both"

autoapi_type = "python"
autoapi_dirs = ["../../slidge_whatsapp"]
autoapi_ignore = ["generated/*"]

intersphinx_mapping = {
    "python": ("https://docs.python.org/3", None),
    "slixmpp": ("https://slixmpp.readthedocs.io/en/latest/", None),
    "slidge": ("https://slidge.im/core/", None),
}

extlinks = {"xep": ("https://xmpp.org/extensions/xep-%s.html", "XEP-%s")}
