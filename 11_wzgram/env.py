import os

API_ID = int(os.getenv("API_ID", "0"))
API_HASH = os.getenv("API_HASH", "")
AUTH_STRING = os.getenv("AUTH_STRING", "")
MESSAGE_LINK = os.getenv("MESSAGE_LINK", "")
CHAT_ID = int(os.getenv("CHAT_ID", "0"))

if not API_ID:
    raise ValueError("Invalid API_ID")
if not API_HASH:
    raise ValueError("API_HASH not set")
if not AUTH_STRING:
    raise ValueError("AUTH_STRING not set")
if not CHAT_ID:
    raise ValueError("CHAT_ID not set")
if not MESSAGE_LINK:
    raise ValueError("MESSAGE_LINK not set")
