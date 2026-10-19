import os

API_ID = int(os.getenv("API_ID", "0"))
API_HASH = os.getenv("API_HASH", "")
AUTH_STRING = os.getenv("AUTH_STRING", "")
MESSAGE_LINK = os.getenv("MESSAGE_LINK", "")
CHAT_ID = int(os.getenv("CHAT_ID", "0"))
EXPORT_AUTH_STRING = os.getenv("EXPORT_AUTH_STRING")
BOT_TOKEN = os.getenv("BOT_TOKEN")

if API_ID <= 0:
    raise ValueError("API_ID must be a positive integer")
if not API_HASH:
    raise ValueError("API_HASH not set")
if EXPORT_AUTH_STRING:
    if not BOT_TOKEN:
        raise ValueError("BOT_TOKEN not set")
else:
    if not AUTH_STRING:
        raise ValueError("AUTH_STRING not set")
    if not CHAT_ID:
        raise ValueError("CHAT_ID not set")
    if not MESSAGE_LINK:
        raise ValueError("MESSAGE_LINK not set")
