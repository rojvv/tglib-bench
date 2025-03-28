import sys
import time
import json

from telethon import __version__  # type: ignore
from telethon.sessions import MemorySession, StringSession  # type: ignore
from telethon.sync import TelegramClient  # type: ignore

from FastTelethon import download_file, upload_file

import env
from util import parse_message_link


app = TelegramClient(
    session=StringSession(None if env.EXPORT_AUTH_STRING else env.AUTH_STRING),
    api_id=env.API_ID,
    api_hash=env.API_HASH,
    flood_sleep_threshold=100,
    receive_updates=False,
)
app.connect()

if env.EXPORT_AUTH_STRING:
    app.sign_in(bot_token=env.BOT_TOKEN)
    print(app.session.save())
    exit(0)

chat_id, message_id = parse_message_link(env.MESSAGE_LINK)

timestamps = list[float]()
message = app.get_messages(entity=chat_id, ids=message_id)
if not message:
    print("Message not found.", file=sys.stderr)
    exit(1)
if not message.file:
    print("Invalid message.", file=sys.stderr)
    exit(1)

file_size = message.file.size

timestamps.append(time.time())

# file_name = message.download_media()
file_name = "file.xxx"
download_file(app, message.document, out=open(file_name, "wb"))

timestamps.append(time.time())

timestamps.append(time.time())

file = upload_file(app, open(file_name, "rb"))
file.name = file_name

app.send_file(entity=env.CHAT_ID, file=file_name, force_document=True)

# app.send_file(
#     entity=env.CHAT_ID,
#     file=file_name,
#     force_document=True,
# )

timestamps.append(time.time())

app.disconnect()

with open("results.json", "w+") as f:
    json.dump([file_size, timestamps, __version__], f)
