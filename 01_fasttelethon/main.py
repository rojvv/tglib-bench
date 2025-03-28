import sys
import time
import json
import asyncio
from io import BytesIO

from telethon import __version__
from telethon.sessions import StringSession
from telethon import TelegramClient

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


async def main():
    await app.connect()

    if env.EXPORT_AUTH_STRING:
        await app.sign_in(bot_token=env.BOT_TOKEN)
        print(app.session.save())
        exit(0)

    chat_id, message_id = parse_message_link(env.MESSAGE_LINK)

    timestamps = list[float]()
    message = await app.get_messages(entity=chat_id, ids=message_id)
    if not message:
        print("Message not found.", file=sys.stderr)
        exit(1)
    if not message.file:
        print("Invalid message.", file=sys.stderr)
        exit(1)

    file_size = message.file.size

    buffer = BytesIO()
    buffer.name = "file"
    timestamps.append(time.time())
    _ = await download_file(app, message.document, out=buffer)
    timestamps.append(time.time())

    buffer.seek(0)
    timestamps.append(time.time())
    file_ = await upload_file(app, buffer, file_size)
    timestamps.append(time.time())

    await app.send_file(entity=env.CHAT_ID, file=file_, force_document=True)

    await app.disconnect()

    with open("results.json", "w+") as f:
        json.dump([file_size, timestamps, __version__], f)


asyncio.run(main())
