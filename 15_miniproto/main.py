import json
import tempfile
import time
from importlib.metadata import version
from pathlib import Path

from miniproto import Client, ClientConfig, InMemorySessionStorage, event_loop
from miniproto.peers import input_channel_from_peer
from miniproto.raw import functions, types

from util import parse_message_link


async def main():
    import env

    app = Client(ClientConfig(api_id=env.API_ID, api_hash=env.API_HASH, session_storage=InMemorySessionStorage(), flood_sleep_threshold=100))
    if not env.EXPORT_AUTH_STRING:
        await app.import_session_string(env.AUTH_STRING)

    async with app:
        if env.EXPORT_AUTH_STRING:
            await app.sign_in_bot(env.BOT_TOKEN)
            print(await app.export_session_string())
            return

        chat_id, message_id = parse_message_link(env.MESSAGE_LINK)
        peer = await app.resolve_peer(chat_id)
        ids = (types.InputMessageID(id=message_id),)
        if peer.kind == "channel":
            request = functions.ChannelsGetMessages(channel=input_channel_from_peer(peer), id=ids)
        else:
            request = functions.MessagesGetMessages(id=ids)
        result = await app.invoke(request)
        message = next((message for message in result.messages if isinstance(message, types.Message) and message.id == message_id), None)
        if message is None:
            raise ValueError("Message not found")
        if not isinstance(message.media, types.MessageMediaDocument) or not isinstance(message.media.document, types.Document):
            raise TypeError("Message must contain a document")
        file_size = message.media.document.size

        with tempfile.TemporaryDirectory(prefix="miniproto-bench-") as directory:
            file_name = Path(directory) / "file"
            timestamps = [time.monotonic()]
            downloaded = await app.download_media(message, file_name)
            timestamps.append(time.monotonic())
            if downloaded.bytes_downloaded != file_size:
                raise ValueError("Downloaded file size does not match the document size")

            timestamps.append(time.monotonic())
            await app.send_file(env.CHAT_ID, file_name, force_file=True)
            timestamps.append(time.monotonic())

    with open("results.json", "w", encoding="utf-8") as file:
        json.dump([file_size, timestamps, version("miniproto")], file)


if __name__ == "__main__":
    event_loop.run(main())
