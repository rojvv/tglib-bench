import json
import sys
import time

from pyrogram import Client
from pyrogram import __version__

import env
from util import parse_message_link

BOT_TOKEN = env.BOT_TOKEN
API_ID = env.API_ID
API_HASH = env.API_HASH
CHAT_ID = env.CHAT_ID
MESSAGE_LINK = env.MESSAGE_LINK


async def main():
    app = Client(
        ":memory:",
        bot_token=BOT_TOKEN,
        api_id=API_ID,
        api_hash=API_HASH,
        no_updates=True,
    )
    await app.start()

    chat_id, message_id = parse_message_link(MESSAGE_LINK)

    timestamps: list[float] = []
    message = await app.get_messages(chat_id=chat_id, message_ids=message_id)
    if not message:
        print("Message not found.", file=sys.stderr)
        await app.stop()
        sys.exit(1)

    if not message.document:
        print("Invalid message.", file=sys.stderr)
        await app.stop()
        sys.exit(1)

    file_size = message.document.file_size

    timestamps.append(time.time())
    file_name = await message.download()
    print(f"Downloaded {file_name}", file=sys.stderr)
    timestamps.append(time.time())

    timestamps.append(time.time())
    print("\nUploading...")
    await app.send_document(
        chat_id=CHAT_ID,
        document=file_name,
        force_document=True,
    )
    timestamps.append(time.time())

    with open("results.json", "w+") as f:
        json.dump([file_size, timestamps, __version__], f)

    await app.stop()


if __name__ == "__main__":
    Client("", no_updates=True).run(main())
