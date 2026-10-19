from urllib.parse import urlparse


def parse_message_link(message_link: str) -> tuple[str | int, int]:
    url = urlparse(message_link)
    parts = url.path.strip("/").split("/")
    if url.scheme not in {"http", "https"} or url.hostname not in {"t.me", "telegram.me"}:
        raise ValueError("MESSAGE_LINK must be a Telegram message URL")
    if len(parts) == 3 and parts[0] == "c":
        channel_id = int(parts[1])
        if channel_id <= 0:
            raise ValueError("MESSAGE_LINK channel ID must be positive")
        chat_id: str | int = -1000000000000 - channel_id
    elif len(parts) == 2 and parts[0] not in {"c", "joinchat", "s"}:
        chat_id = parts[0]
    else:
        raise ValueError("MESSAGE_LINK must use /username/message_id or /c/channel_id/message_id")
    message_id = int(parts[-1])
    if message_id <= 0:
        raise ValueError("MESSAGE_LINK message ID must be positive")
    return chat_id, message_id
