from typing import TYPE_CHECKING, Optional

from slidge.core.command import Command, CommandAccess, Form, FormField
from slixmpp import JID

from .generated import whatsapp

if TYPE_CHECKING:
    from .session import Session


class ChangePresence(Command):
    NAME = "Set WhatsApp web presence"
    HELP = (
        "If you want to receive notifications in the WhatsApp official client,"
        "you need to set your presence to unavailable. As a side effect, you "
        "won't receive receipts and presences from your contacts."
    )
    NODE = "wa_presence"
    CHAT_COMMAND = "presence"
    ACCESS = CommandAccess.USER_LOGGED

    async def run(
        self,
        session: Optional["Session"],  # type:ignore
        ifrom: JID,
        *args,
    ) -> Form:
        return Form(
            title="Set WhatsApp web presence",
            instructions="Choose what type of presence you want to set",
            fields=[
                FormField(
                    var="presence",
                    value="available",
                    type="list-single",
                    options=[
                        {"label": "Available", "value": "available"},
                        {"label": "Unavailable", "value": "unavailable"},
                    ],
                )
            ],
            handler=self.finish,
        )

    @staticmethod
    async def finish(form_values: dict[str, str], session: "Session", _ifrom: JID):
        p = form_values.get("presence")
        if p == "available":
            session.whatsapp.SendPresence(whatsapp.PresenceAvailable)
        elif p == "unavailable":
            session.whatsapp.SendPresence(whatsapp.PresenceUnavailable)
        else:
            raise ValueError("Not a valid presence kind.", p)
        return f"Presence succesfully set to {p}"


class SubscribeToPresences(Command):
    NAME = "Subscribe to contacts' presences"
    HELP = (
        "This command is here for tests about "
        "https://todo.sr.ht/~nicoco/slidge-whatsapp/7 ."
    )
    NODE = "wa_subscribe"
    CHAT_COMMAND = "subscribe"
    ACCESS = CommandAccess.USER_LOGGED

    async def run(
        self,
        session: Optional["Session"],  # type:ignore
        ifrom: JID,
        *args,
    ) -> str:
        assert session is not None
        session.whatsapp.GetContacts(False)
        return "Looks like no exception was raised. Success, I guess?"
