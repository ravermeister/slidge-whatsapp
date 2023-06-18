Configuration
=============

Setting up a slidge component
-----------------------------

Refer to the `slidge admin docs <https://slidge.im/admin>`_ for generic
instructions on how to set up a slidge component, and for slidge core
configuration options.

slidge-whatsapp-specific config
-------------------------------

Set the environment variable ``SLIDGE_WA_ATTACHMENTS_ON_DISK``
(to any value) as a workaround for high CPU usage on incoming attachments
(`reference <https://github.com/go-python/gopy/issues/323>`_).

.. config-obj:: slidge_whatsapp.config
