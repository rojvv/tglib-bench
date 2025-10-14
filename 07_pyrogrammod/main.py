import json
import sys
import time

from pyrogram import Client
from pyrogram import __version__

import env
from util import parse_message_link

app = Client(
    "session_user",
    api_id=env.API_ID,
    api_hash=env.API_HASH,
    bot_token=env.BOT_TOKEN,
    no_updates=True,
)


async def main():
    await app.start()

    chat_id, message_id = parse_message_link(env.MESSAGE_LINK)

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
    file_name = await message.download(use_experimental_download_boost=True)
    print(f"Downloaded {file_name}", file=sys.stderr)
    timestamps.append(time.time())

    timestamps.append(time.time())
    print("\nUploading...")
    await app.send_document(
        chat_id=env.CHAT_ID,
        document=file_name,
        force_document=True,
        # TODO: use_experimental_upload_boost=True,  # Not yet implemented
    )
    timestamps.append(time.time())

    with open("results.json", "w+") as f:
        json.dump([file_size, timestamps, __version__], f)


if __name__ == "__main__":
    app.run(main())
