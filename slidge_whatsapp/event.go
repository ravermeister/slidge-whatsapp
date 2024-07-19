package whatsapp

import (
	// Standard library.
	"context"
	"fmt"
	"mime"
	"os"
	"strings"

	// Third-party libraries.
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// EventKind represents all event types recognized by the Python session adapter, as emitted by the
// Go session adapter.
type EventKind int

// The event types handled by the overarching session adapter handler.
const (
	EventUnknown EventKind = iota
	EventQRCode
	EventPair
	EventConnected
	EventLoggedOut
	EventContact
	EventPresence
	EventMessage
	EventChatState
	EventReceipt
	EventGroup
	EventCall
)

// EventPayload represents the collected payloads for all event types handled by the overarching
// session adapter handler. Only specific fields will be populated in events emitted by internal
// handlers, see documentation for specific types for more information.
type EventPayload struct {
	QRCode       string
	PairDeviceID string
	ConnectedJID string
	Contact      Contact
	Presence     Presence
	Message      Message
	ChatState    ChatState
	Receipt      Receipt
	Group        Group
	Call         Call
}

// A Avatar represents a small image set for a Contact or Group.
type Avatar struct {
	ID  string // The unique ID for this avatar, used for persistent caching.
	URL string // The HTTP URL over which this avatar might be retrieved. Can change for the same ID.
}

// A Contact represents any entity that be communicated with directly in WhatsApp. This typically
// represents people, but may represent a business or bot as well, but not a group-chat.
type Contact struct {
	JID  string // The WhatsApp JID for this contact.
	Name string // The user-set, human-readable name for this contact.
}

// NewContactEvent returns event data meant for [Session.propagateEvent] for the contact information
// given. Unknown or invalid contact information will return an [EventUnknown] event with nil data.
func newContactEvent(jid types.JID, info types.ContactInfo) (EventKind, *EventPayload) {
	var contact = Contact{
		JID: jid.ToNonAD().String(),
	}

	for _, n := range []string{info.FullName, info.FirstName, info.BusinessName, info.PushName} {
		if n != "" {
			contact.Name = n
			break
		}
	}

	// Don't attempt to synchronize contacts with no user-readable name.
	if contact.Name == "" {
		return EventUnknown, nil
	}

	return EventContact, &EventPayload{Contact: contact}
}

// PresenceKind represents the different kinds of activity states possible in WhatsApp.
type PresenceKind int

// The presences handled by the overarching session event handler.
const (
	PresenceAvailable PresenceKind = 1 + iota
	PresenceUnavailable
)

// Precence represents a contact's general state of activity, and is periodically updated as
// contacts start or stop paying attention to their client of choice.
type Presence struct {
	JID      string
	Kind     PresenceKind
	LastSeen int64
}

// NewPresenceEvent returns event data meant for [Session.propagateEvent] for the primitive presence
// event given.
func newPresenceEvent(evt *events.Presence) (EventKind, *EventPayload) {
	var presence = Presence{
		JID:      evt.From.ToNonAD().String(),
		Kind:     PresenceAvailable,
		LastSeen: evt.LastSeen.Unix(),
	}

	if evt.Unavailable {
		presence.Kind = PresenceUnavailable
	}

	return EventPresence, &EventPayload{Presence: presence}
}

// MessageKind represents all concrete message types (plain-text messages, edit messages, reactions)
// recognized by the Python session adapter.
type MessageKind int

// The message types handled by the overarching session event handler.
const (
	MessagePlain MessageKind = 1 + iota
	MessageEdit
	MessageRevoke
	MessageReaction
	MessageAttachment
)

// A Message represents one of many kinds of bidirectional communication payloads, for example, a
// text message, a file (image, video) attachment, an emoji reaction, etc. Messages of different
// kinds are denoted as such, and re-use fields where the semantics overlap.
type Message struct {
	Kind        MessageKind  // The concrete message kind being sent or received.
	ID          string       // The unique message ID, used for referring to a specific Message instance.
	JID         string       // The JID this message concerns, semantics can change based on IsCarbon.
	GroupJID    string       // The JID of the group-chat this message was sent in, if any.
	OriginJID   string       // For reactions and replies in groups, the JID of the original user.
	Body        string       // The plain-text message body. For attachment messages, this can be a caption.
	Timestamp   int64        // The Unix timestamp denoting when this message was created.
	IsCarbon    bool         // Whether or not this message concerns the gateway user themselves.
	ReplyID     string       // The unique message ID this message is in reply to, if any.
	ReplyBody   string       // The full body of the message this message is in reply to, if any.
	Attachments []Attachment // The list of file (image, video, etc.) attachments contained in this message.
	Preview     Preview      // A short description for the URL provided in the message body, if any.
	MentionJIDs []string     // A list of JIDs mentioned in this message, if any.
	Receipts    []Receipt    // The receipt statuses for the message, typically provided alongside historical messages.
	Reactions   []Message    // Reactions attached to message, typically provided alongside historical messages.
}

// A Attachment represents additional binary data (e.g. images, videos, documents) provided alongside
// a message, for display or storage on the recepient client.
type Attachment struct {
	MIME     string // The MIME type for attachment.
	Filename string // The recommended file name for this attachment. May be an auto-generated name.
	Caption  string // The user-provided caption, provided alongside this attachment.
	Path     string // Local path to the file is stored on disk.

	// Internal fields.
	meta attachmentMetadata // Metadata specific to audio/video files, used in processing.
}

// A Preview represents a short description for a URL provided in a message body, as usually derived
// from the content of the page pointed at.
type Preview struct {
	URL         string // The original (or canonical) URL this preview was generated for.
	Title       string // The short title for the URL preview.
	Description string // The (optional) long-form description for the URL preview.
	ImagePath   string // The local path for the image associated with the URL.
}

// NewMessageEvent returns event data meant for [Session.propagateEvent] for the primive message
// event given. Unknown or invalid messages will return an [EventUnknown] event with nil data.
func newMessageEvent(client *whatsmeow.Client, evt *events.Message) (EventKind, *EventPayload) {
	// Set basic data for message, to be potentially amended depending on the concrete version of
	// the underlying message.
	var message = Message{
		Kind:      MessagePlain,
		ID:        evt.Info.ID,
		JID:       evt.Info.Sender.ToNonAD().String(),
		Body:      evt.Message.GetConversation(),
		Timestamp: evt.Info.Timestamp.Unix(),
		IsCarbon:  evt.Info.IsFromMe,
	}

	// Broadcast and status messages are currently not handled at all.
	if evt.Info.Chat.Server == types.BroadcastServer {
		return EventUnknown, nil
	}

	if evt.Info.IsGroup {
		message.GroupJID = evt.Info.Chat.ToNonAD().String()
	} else if message.IsCarbon {
		message.JID = evt.Info.Chat.ToNonAD().String()
	}

	// Handle handle protocol messages (such as message deletion or editing).
	if p := evt.Message.GetProtocolMessage(); p != nil {
		switch p.GetType() {
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			if m := p.GetEditedMessage(); m != nil {
				message.Kind = MessageEdit
				message.ID = p.Key.GetID()
				message.Body = m.GetConversation()
			} else {
				return EventUnknown, nil
			}
		case waE2E.ProtocolMessage_REVOKE:
			message.Kind = MessageRevoke
			message.ID = p.Key.GetID()
			message.OriginJID = p.Key.GetParticipant()
			return EventMessage, &EventPayload{Message: message}
		}
	}

	// Handle emoji reaction to existing message.
	if r := evt.Message.GetReactionMessage(); r != nil {
		message.Kind = MessageReaction
		message.ID = r.Key.GetID()
		message.Body = r.GetText()
		return EventMessage, &EventPayload{Message: message}
	}

	// Handle message attachments, if any.
	if attach, context, err := getMessageAttachments(client, evt.Message); err != nil {
		client.Log.Errorf("Failed getting message attachments: %s", err)
		return EventUnknown, nil
	} else if len(attach) > 0 {
		message.Attachments = append(message.Attachments, attach...)
		message.Kind = MessageAttachment
		if context != nil {
			message = getMessageWithContext(message, context)
		}
	}

	// Get extended information from message, if available. Extended messages typically represent
	// messages with additional context, such as replies, forwards, etc.
	if e := evt.Message.GetExtendedTextMessage(); e != nil {
		if message.Body == "" {
			message.Body = e.GetText()
		}

		message = getMessageWithContext(message, e.GetContextInfo())
	}

	// Ignore obviously invalid messages.
	if message.Kind == MessagePlain && message.Body == "" {
		return EventUnknown, nil
	}

	return EventMessage, &EventPayload{Message: message}
}

// GetMessageWithContext processes the given [Message] and applies any context metadata might be
// useful; examples of context include messages being quoted. If no context is found, the original
// message is returned unchanged.
func getMessageWithContext(message Message, info *waE2E.ContextInfo) Message {
	if info == nil {
		return message
	}

	message.ReplyID = info.GetStanzaID()
	message.OriginJID = info.GetParticipant()

	if q := info.GetQuotedMessage(); q != nil {
		if qe := q.GetExtendedTextMessage(); qe != nil {
			message.ReplyBody = qe.GetText()
		} else {
			message.ReplyBody = q.GetConversation()
		}
	}

	return message
}

// GetMessageAttachments fetches and decrypts attachments (images, audio, video, or documents) sent
// via WhatsApp. Any failures in retrieving any attachment will return an error immediately.
func getMessageAttachments(client *whatsmeow.Client, message *waE2E.Message) ([]Attachment, *waE2E.ContextInfo, error) {
	var result []Attachment
	var context *waE2E.ContextInfo
	var kinds = []whatsmeow.DownloadableMessage{
		message.GetImageMessage(),
		message.GetAudioMessage(),
		message.GetVideoMessage(),
		message.GetDocumentMessage(),
		message.GetStickerMessage(),
	}

	for _, msg := range kinds {
		// Handle data for specific attachment type.
		var a Attachment
		switch msg := msg.(type) {
		case *waE2E.ImageMessage:
			a.MIME, a.Caption = msg.GetMimetype(), msg.GetCaption()
		case *waE2E.AudioMessage:
			a.MIME = msg.GetMimetype()
		case *waE2E.VideoMessage:
			a.MIME, a.Caption = msg.GetMimetype(), msg.GetCaption()
		case *waE2E.DocumentMessage:
			a.MIME, a.Caption, a.Filename = msg.GetMimetype(), msg.GetCaption(), msg.GetFileName()
		case *waE2E.StickerMessage:
			a.MIME = msg.GetMimetype()
		}

		// Ignore attachments with empty or unknown MIME types.
		if a.MIME == "" {
			continue
		}

		// Set filename from SHA256 checksum and MIME type, if none is already set.
		if a.Filename == "" {
			a.Filename = fmt.Sprintf("%x%s", msg.GetFileSHA256(), extensionByType(a.MIME))
		}

		// Attempt to download and decrypt raw attachment data, if any.
		data, err := client.Download(msg)
		if err != nil {
			return nil, nil, err
		}

		tmp, err := createTempFile(data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed writing to temporary file: %w", err)
		}

		a.Path = tmp
		result = append(result, a)
	}

	// Handle any contact vCard as attachment.
	if c := message.GetContactMessage(); c != nil {
		tmp, err := createTempFile([]byte(c.GetVcard()))
		if err != nil {
			return nil, nil, fmt.Errorf("Failed getting contact message: %w", err)
		}
		result = append(result, Attachment{
			MIME:     "text/vcard",
			Filename: c.GetDisplayName() + ".vcf",
			Path:     tmp,
		})
		context = c.GetContextInfo()
	}

	return result, context, nil
}

// KnownMediaTypes represents MIME type to WhatsApp media types known to be handled by WhatsApp in a
// special way (that is, not as generic file uploads).
var knownMediaTypes = map[string]whatsmeow.MediaType{
	"image/jpeg": whatsmeow.MediaImage,
	"audio/mpeg": whatsmeow.MediaAudio,
	"audio/mp4":  whatsmeow.MediaAudio,
	"audio/aac":  whatsmeow.MediaAudio,
	"audio/ogg":  whatsmeow.MediaAudio,
	"video/mp4":  whatsmeow.MediaVideo,
}

// UploadAttachment attempts to push the given attachment data to WhatsApp according to the MIME
// type specified within. Attachments are handled as generic file uploads unless they're of a
// specific format; in addition, certain MIME types may be automatically converted to a
// well-supported type via FFmpeg (if available).
func uploadAttachment(client *whatsmeow.Client, attach *Attachment) (*waE2E.Message, error) {
	var originalMIME = attach.MIME
	if err := convertAttachment(attach); err != nil {
		client.Log.Warnf("failed to auto-convert attachment: %s", err)
	}

	mediaType := knownMediaTypes[strings.Split(attach.MIME, ";")[0]]
	if mediaType == "" {
		mediaType = whatsmeow.MediaDocument
	}

	data, err := os.ReadFile(attach.Path)
	if err != nil {
		return nil, err
	} else if len(data) == 0 {
		return nil, fmt.Errorf("attachment file contains no data")
	}

	upload, err := client.Upload(context.Background(), data, mediaType)
	if err != nil {
		return nil, err
	}

	var message *waE2E.Message
	switch mediaType {
	case whatsmeow.MediaImage:
		message = &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &upload.URL,
				DirectPath:    &upload.DirectPath,
				MediaKey:      upload.MediaKey,
				Mimetype:      &attach.MIME,
				FileEncSHA256: upload.FileEncSHA256,
				FileSHA256:    upload.FileSHA256,
				FileLength:    ptrTo(uint64(len(data))),
			},
		}
	case whatsmeow.MediaAudio:
		if attach.meta == (attachmentMetadata{}) {
			if err = populateAttachmentMetadata(attach); err != nil {
				client.Log.Warnf("failed fetching attachment metadata: %s", err)
			}
		}
		message = &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				URL:           &upload.URL,
				DirectPath:    &upload.DirectPath,
				MediaKey:      upload.MediaKey,
				Mimetype:      &attach.MIME,
				FileEncSHA256: upload.FileEncSHA256,
				FileSHA256:    upload.FileSHA256,
				FileLength:    ptrTo(uint64(len(data))),
				Seconds:       ptrTo(uint32(attach.meta.duration.Seconds())),
			},
		}
		if attach.MIME == voiceMessageMIME {
			message.AudioMessage.PTT = ptrTo(true)
			if wave, err := getAttachmentWaveform(attach); err != nil {
				client.Log.Warnf("failed generating attachment waveform: %s", err)
			} else {
				message.AudioMessage.Waveform = wave
			}
		}
	case whatsmeow.MediaVideo:
		if attach.meta == (attachmentMetadata{}) {
			if err = populateAttachmentMetadata(attach); err != nil {
				client.Log.Warnf("failed fetching attachment metadata: %s", err)
			}
		}
		message = &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				URL:           &upload.URL,
				DirectPath:    &upload.DirectPath,
				MediaKey:      upload.MediaKey,
				Mimetype:      &attach.MIME,
				FileEncSHA256: upload.FileEncSHA256,
				FileSHA256:    upload.FileSHA256,
				FileLength:    ptrTo(uint64(len(data))),
				Seconds:       ptrTo(uint32(attach.meta.duration.Seconds())),
				Width:         ptrTo(uint32(attach.meta.width)),
				Height:        ptrTo(uint32(attach.meta.height)),
			}}
		if thumb, err := getAttachmentThumbnail(attach); err != nil {
			client.Log.Warnf("failed generating attachment thumbnail: %s", err)
		} else {
			message.VideoMessage.JPEGThumbnail = thumb
		}
		if originalMIME == animatedImageMIME {
			message.VideoMessage.GifPlayback = ptrTo(true)
		}
	case whatsmeow.MediaDocument:
		message = &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				URL:           &upload.URL,
				DirectPath:    &upload.DirectPath,
				MediaKey:      upload.MediaKey,
				Mimetype:      &attach.MIME,
				FileEncSHA256: upload.FileEncSHA256,
				FileSHA256:    upload.FileSHA256,
				FileLength:    ptrTo(uint64(len(data))),
				FileName:      &attach.Filename,
			}}
	}

	return message, nil
}

// KnownExtensions represents MIME type to file-extension mappings for basic, known media types.
var knownExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"audio/ogg":  ".oga",
	"video/mp4":  ".mp4",
}

// ExtensionByType returns the file extension for the given MIME type, or a generic extension if the
// MIME type is unknown.
func extensionByType(typ string) string {
	// Handle common, known MIME types first.
	if ext := knownExtensions[typ]; ext != "" {
		return ext
	}
	if ext, _ := mime.ExtensionsByType(typ); len(ext) > 0 {
		return ext[0]
	}
	return ".bin"
}

// NewEventFromHistory returns event data meant for [Session.propagateEvent] for the primive history
// message given. Currently, only events related to group-chats will be handled, due to uncertain
// support for history back-fills on 1:1 chats.
//
// Otherwise, the implementation largely follows that of [newMessageEvent], however the base types
// used by these two functions differ in many small ways which prevent unifying the approach.
//
// Typically, this will return [EventMessage] events with appropriate [Message] payloads; unknown or
// invalid messages will return an [EventUnknown] event with nil data.
func newEventFromHistory(client *whatsmeow.Client, info *waWeb.WebMessageInfo) (EventKind, *EventPayload) {
	// Handle message as group message is remote JID is a group JID in the absence of any other,
	// specific signal, or don't handle at all if no group JID is found.
	var jid = info.GetKey().GetRemoteJID()
	if j, _ := types.ParseJID(jid); j.Server != types.GroupServer {
		return EventUnknown, nil
	}

	// Set basic data for message, to be potentially amended depending on the concrete version of
	// the underlying message.
	var message = Message{
		Kind:      MessagePlain,
		ID:        info.GetKey().GetID(),
		GroupJID:  info.GetKey().GetRemoteJID(),
		Body:      info.GetMessage().GetConversation(),
		Timestamp: int64(info.GetMessageTimestamp()),
		IsCarbon:  info.GetKey().GetFromMe(),
	}

	if info.Participant != nil {
		message.JID = info.GetParticipant()
	} else if info.GetKey().GetFromMe() {
		message.JID = client.Store.ID.ToNonAD().String()
	} else {
		// It's likely we cannot handle this message correctly if we don't know the concrete
		// sender, so just ignore it completely.
		return EventUnknown, nil
	}

	// Handle handle protocol messages (such as message deletion or editing), while ignoring known
	// unhandled types.
	switch info.GetMessageStubType() {
	case waWeb.WebMessageInfo_CIPHERTEXT:
		return EventUnknown, nil
	case waWeb.WebMessageInfo_CALL_MISSED_VOICE, waWeb.WebMessageInfo_CALL_MISSED_VIDEO:
		return EventCall, &EventPayload{Call: Call{
			State:     CallMissed,
			JID:       info.GetKey().GetRemoteJID(),
			Timestamp: int64(info.GetMessageTimestamp()),
		}}
	case waWeb.WebMessageInfo_REVOKE:
		if p := info.GetMessageStubParameters(); len(p) > 0 {
			message.Kind = MessageRevoke
			message.ID = p[0]
			return EventMessage, &EventPayload{Message: message}
		} else {
			return EventUnknown, nil
		}
	}

	// Handle emoji reaction to existing message.
	for _, r := range info.GetReactions() {
		if r.GetText() != "" {
			message.Reactions = append(message.Reactions, Message{
				Kind:      MessageReaction,
				ID:        r.GetKey().GetID(),
				JID:       r.GetKey().GetRemoteJID(),
				Body:      r.GetText(),
				Timestamp: r.GetSenderTimestampMS() / 1000,
				IsCarbon:  r.GetKey().GetFromMe(),
			})
		}
	}

	// Handle message attachments, if any.
	if attach, context, err := getMessageAttachments(client, info.GetMessage()); err != nil {
		client.Log.Errorf("Failed getting message attachments: %s", err)
		return EventUnknown, nil
	} else if len(attach) > 0 {
		message.Attachments = append(message.Attachments, attach...)
		message.Kind = MessageAttachment
		if context != nil {
			message = getMessageWithContext(message, context)
		}
	}

	// Handle pre-set receipt status, if any.
	for _, r := range info.GetUserReceipt() {
		// Ignore self-receipts for the moment, as these cannot be handled correctly by the adapter.
		if client.Store.ID.ToNonAD().String() == r.GetUserJID() {
			continue
		}
		var receipt = Receipt{MessageIDs: []string{message.ID}, JID: r.GetUserJID(), GroupJID: message.GroupJID}
		switch info.GetStatus() {
		case waWeb.WebMessageInfo_DELIVERY_ACK:
			receipt.Kind = ReceiptDelivered
			receipt.Timestamp = r.GetReceiptTimestamp()
		case waWeb.WebMessageInfo_READ:
			receipt.Kind = ReceiptRead
			receipt.Timestamp = r.GetReadTimestamp()
		}
		message.Receipts = append(message.Receipts, receipt)
	}

	// Get extended information from message, if available. Extended messages typically represent
	// messages with additional context, such as replies, forwards, etc.
	if e := info.GetMessage().GetExtendedTextMessage(); e != nil {
		if message.Body == "" {
			message.Body = e.GetText()
		}

		message = getMessageWithContext(message, e.GetContextInfo())
	}

	// Ignore obviously invalid messages.
	if message.Kind == MessagePlain && message.Body == "" {
		return EventUnknown, nil
	}

	return EventMessage, &EventPayload{Message: message}
}

// ChatStateKind represents the different kinds of chat-states possible in WhatsApp.
type ChatStateKind int

// The chat states handled by the overarching session event handler.
const (
	ChatStateComposing ChatStateKind = 1 + iota
	ChatStatePaused
)

// A ChatState represents the activity of a contact within a certain discussion, for instance,
// whether the contact is currently composing a message. This is separate to the concept of a
// Presence, which is the contact's general state across all discussions.
type ChatState struct {
	Kind     ChatStateKind
	JID      string
	GroupJID string
}

// NewChatStateEvent returns event data meant for [Session.propagateEvent] for the primitive
// chat-state event given.
func newChatStateEvent(evt *events.ChatPresence) (EventKind, *EventPayload) {
	var state = ChatState{JID: evt.MessageSource.Sender.ToNonAD().String()}
	if evt.MessageSource.IsGroup {
		state.GroupJID = evt.MessageSource.Chat.ToNonAD().String()
	}
	switch evt.State {
	case types.ChatPresenceComposing:
		state.Kind = ChatStateComposing
	case types.ChatPresencePaused:
		state.Kind = ChatStatePaused
	}
	return EventChatState, &EventPayload{ChatState: state}
}

// ReceiptKind represents the different types of delivery receipts possible in WhatsApp.
type ReceiptKind int

// The delivery receipts handled by the overarching session event handler.
const (
	ReceiptDelivered ReceiptKind = 1 + iota
	ReceiptRead
)

// A Receipt represents a notice of delivery or presentation for [Message] instances sent or
// received. Receipts can be delivered for many messages at once, but are generally all delivered
// under one specific state at a time.
type Receipt struct {
	Kind       ReceiptKind // The distinct kind of receipt presented.
	MessageIDs []string    // The list of message IDs to mark for receipt.
	JID        string
	GroupJID   string
	Timestamp  int64
	IsCarbon   bool
}

// NewReceiptEvent returns event data meant for [Session.propagateEvent] for the primive receipt
// event given. Unknown or invalid receipts will return an [EventUnknown] event with nil data.
func newReceiptEvent(evt *events.Receipt) (EventKind, *EventPayload) {
	var receipt = Receipt{
		MessageIDs: append([]string{}, evt.MessageIDs...),
		JID:        evt.MessageSource.Sender.ToNonAD().String(),
		Timestamp:  evt.Timestamp.Unix(),
		IsCarbon:   evt.MessageSource.IsFromMe,
	}

	if len(receipt.MessageIDs) == 0 {
		return EventUnknown, nil
	}

	// Receipts for broadcast and status messages are currently not handled at all.
	if evt.MessageSource.Chat.Server == types.BroadcastServer {
		return EventUnknown, nil
	}

	if evt.MessageSource.IsGroup {
		receipt.GroupJID = evt.MessageSource.Chat.ToNonAD().String()
	} else if receipt.IsCarbon {
		receipt.JID = evt.MessageSource.Chat.ToNonAD().String()
	}

	switch evt.Type {
	case types.ReceiptTypeDelivered:
		receipt.Kind = ReceiptDelivered
	case types.ReceiptTypeRead:
		receipt.Kind = ReceiptRead
	}

	return EventReceipt, &EventPayload{Receipt: receipt}
}

// GroupAffiliation represents the set of privilidges given to a specific participant in a group.
type GroupAffiliation int

const (
	GroupAffiliationNone  GroupAffiliation = iota // None, or normal member group affiliation.
	GroupAffiliationAdmin                         // Can perform some management operations.
	GroupAffiliationOwner                         // Can manage group fully, including destroying the group.
)

// A Group represents a named, many-to-many chat space which may be joined or left at will. All
// fields apart from the group JID are considered to be optional, and may not be set in cases where
// group information is being updated against previous assumed state. Groups in WhatsApp are
// generally invited to out-of-band with respect to overarching adaptor; see the documentation for
// [Session.GetGroups] for more information.
type Group struct {
	JID          string             // The WhatsApp JID for this group.
	Name         string             // The user-defined, human-readable name for this group.
	Subject      GroupSubject       // The longer-form, user-defined description for this group.
	Nickname     string             // Our own nickname in this group-chat.
	Participants []GroupParticipant // The list of participant contacts for this group, including ourselves.
}

// A GroupSubject represents the user-defined group description and attached metadata thereof, for a
// given [Group].
type GroupSubject struct {
	Subject  string // The user-defined group description.
	SetAt    int64  // The exact time this group description was set at, as a timestamp.
	SetByJID string // The JID of the user that set the subject.
}

// GroupParticipantAction represents the distinct set of actions that can be taken when encountering
// a group participant, typically to add or remove.
type GroupParticipantAction int

const (
	GroupParticipantActionAdd    GroupParticipantAction = iota // Default action; add participant to list.
	GroupParticipantActionUpdate                               // Update existing participant information.
	GroupParticipantActionRemove                               // Remove participant from list, if existing.
)

// A GroupParticipant represents a contact who is currently joined in a given group. Participants in
// WhatsApp can always be derived back to their individual [Contact]; there are no anonymous groups
// in WhatsApp.
type GroupParticipant struct {
	JID         string                 // The WhatsApp JID for this participant.
	Affiliation GroupAffiliation       // The set of priviledges given to this specific participant.
	Action      GroupParticipantAction // The specific action to take for this participant; typically to add.
}

// NewGroupEvent returns event data meant for [Session.propagateEvent] for the primive group event
// given. Group data returned by this function can be partial, and callers should take care to only
// handle non-empty values.
func newGroupEvent(evt *events.GroupInfo) (EventKind, *EventPayload) {
	var group = Group{JID: evt.JID.ToNonAD().String()}
	if evt.Name != nil {
		group.Name = evt.Name.Name
	}
	if evt.Topic != nil {
		group.Subject = GroupSubject{
			Subject:  evt.Topic.Topic,
			SetAt:    evt.Topic.TopicSetAt.Unix(),
			SetByJID: evt.Topic.TopicSetBy.ToNonAD().String(),
		}
	}
	for _, p := range evt.Join {
		group.Participants = append(group.Participants, GroupParticipant{
			JID:    p.ToNonAD().String(),
			Action: GroupParticipantActionAdd,
		})
	}
	for _, p := range evt.Leave {
		group.Participants = append(group.Participants, GroupParticipant{
			JID:    p.ToNonAD().String(),
			Action: GroupParticipantActionRemove,
		})
	}
	for _, p := range evt.Promote {
		group.Participants = append(group.Participants, GroupParticipant{
			JID:         p.ToNonAD().String(),
			Action:      GroupParticipantActionUpdate,
			Affiliation: GroupAffiliationAdmin,
		})
	}
	for _, p := range evt.Demote {
		group.Participants = append(group.Participants, GroupParticipant{
			JID:         p.ToNonAD().String(),
			Action:      GroupParticipantActionUpdate,
			Affiliation: GroupAffiliationNone,
		})
	}
	return EventGroup, &EventPayload{Group: group}
}

// NewGroup returns a concrete [Group] for the primitive data given. This function will generally
// populate fields with as much data as is available from the remote, and is therefore should not
// be called when partial data is to be returned.
func newGroup(client *whatsmeow.Client, info *types.GroupInfo) Group {
	var participants []GroupParticipant
	for _, p := range info.Participants {
		if p.Error > 0 {
			continue
		}
		var affiliation = GroupAffiliationNone
		if p.IsSuperAdmin {
			affiliation = GroupAffiliationOwner
		} else if p.IsAdmin {
			affiliation = GroupAffiliationAdmin
		}
		participants = append(participants, GroupParticipant{
			JID:         p.JID.ToNonAD().String(),
			Affiliation: affiliation,
		})
	}
	return Group{
		JID:  info.JID.ToNonAD().String(),
		Name: info.GroupName.Name,
		Subject: GroupSubject{
			Subject:  info.Topic,
			SetAt:    info.TopicSetAt.Unix(),
			SetByJID: info.TopicSetBy.ToNonAD().String(),
		},
		Nickname:     client.Store.PushName,
		Participants: participants,
	}
}

// CallState represents the state of the call to synchronize with.
type CallState int

// The call states handled by the overarching session event handler.
const (
	CallMissed CallState = 1 + iota
)

// A Call represents an incoming or outgoing voice/video call made over WhatsApp. Full support for
// calls is currently not implemented, and this structure contains the bare minimum data required
// for notifying on missed calls.
type Call struct {
	State     CallState
	JID       string
	Timestamp int64
}

// NewCallEvent returns event data meant for [Session.propagateEvent] for the call metadata given.
func newCallEvent(state CallState, meta types.BasicCallMeta) (EventKind, *EventPayload) {
	return EventCall, &EventPayload{Call: Call{
		State:     state,
		JID:       meta.From.ToNonAD().String(),
		Timestamp: meta.Timestamp.Unix(),
	}}
}
