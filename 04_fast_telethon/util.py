
from urllib.parse import urlparse

def parse_message_link(message_link: str) -> tuple[str | int, int]:
    url = urlparse(message_link)
    _, _, chat_id, message_id = (url.path.split("/"))
    try:
        return (-1000000000000 - int(chat_id), int(message_id))
    except ValueError:
        return (chat_id, int(message_id))
