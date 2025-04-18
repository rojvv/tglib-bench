import os

API_ID = int(os.getenv("API_ID", "0"))
API_HASH = os.getenv("API_HASH", "")
AUTH_STRING = os.environ.get("AUTH_STRING", "")
MESSAGE_LINK = os.getenv("MESSAGE_LINK", "")
CHAT_ID = int(os.getenv("CHAT_ID", "0"))

EXPORT_AUTH_STRING = os.getenv("EXPORT_AUTH_STRING")
BOT_TOKEN = os.getenv("BOT_TOKEN")

if not API_ID:
    raise ValueError("Invalid API_ID")
if not API_HASH:
    raise ValueError("API_HASH not set")
if not EXPORT_AUTH_STRING and not AUTH_STRING:
    raise ValueError("AUTH_STRING not set")
if not EXPORT_AUTH_STRING and not CHAT_ID:
    raise ValueError("CHAT_ID not set")
if not EXPORT_AUTH_STRING and not MESSAGE_LINK:
    raise ValueError("MESSAGE_LINK not set")

if EXPORT_AUTH_STRING and not BOT_TOKEN:
    raise ValueError("BOT_TOKEN not set")
