import { ok } from "node:assert";
import { writeFileSync } from "node:fs";
import { Api, TelegramClient, version, sessions } from "teleproto";
import env from "./env.ts";

const client = new TelegramClient(
  new sessions.StringSession(env.AUTH_STRING),
  env.API_ID,
  env.API_HASH,
  {
    connectionRetries: 5,
    floodSleepThreshold: 100,
    maxConcurrentDownloads: 1,
    downloadPool: {
      sessions: 6,
      inflightPerDc: 6,
      partSize: 512 * 1024,
    },
  },
);

await client.connect();
if (!(await client.isBot())) {
  throw new Error("AUTH_STRING is not authorized");
}

const message = await getLinkedMessage(env.MESSAGE_LINK);
ok(message instanceof Api.Message, "Message not found.");
ok(message.document != null, "Message invalid.");

const dates = new Array<Date>();

dates.push(new Date());
const downloaded = await client.downloadMedia(message, {
  progressCallback: progressLogger("Download progress:"),
});
dates.push(new Date());

ok(Buffer.isBuffer(downloaded), "Download did not return a buffer.");

dates.push(new Date());
downloaded.name = "teleproto.bin";
await client.sendFile(env.CHAT_ID, {
  file: downloaded,
  fileSize: downloaded.byteLength,
  forceDocument: true,
  workers: 8,
  progressCallback: (progress)=>{
    console.log('Upload progress:', progress);
  },
  attributes: [
    new Api.DocumentAttributeFilename({ fileName: "teleproto.bin" }),
  ],
});
dates.push(new Date());

await client.disconnect();
writeFileSync(
  "results.json",
  JSON.stringify([
    downloaded.byteLength,
    dates.map((v) => v.getTime() / 1_000),
    version,
  ]),
);

async function getLinkedMessage(link: string): Promise<Api.Message> {
  const parsed = parseMessageLink(link);
  if (parsed.kind === "username") {
    const messages = await client.getMessages(parsed.username, {
      ids: parsed.messageID,
    });
    return firstMessage(messages);
  }

  const channel = await resolvePrivateChannel(parsed.channelID);
  return (await client.getMessages(channel, {ids: [parsed.messageID]}))[0];
}

function firstMessage(messages: Iterable<unknown>): Api.Message {
  for (const message of messages) {
    if (message instanceof Api.Message) {
      return message;
    }
  }
  throw new Error("Message not found.");
}

async function resolvePrivateChannel(
  channelID: string,
): Promise<Api.InputPeerChannel> {
  for await (const dialog of client.iterDialogs({ limit: undefined })) {
    const entity = dialog.entity;
    if (
      (entity instanceof Api.Channel ||
        entity instanceof Api.ChannelForbidden) &&
      entity.id.toString() === channelID
    ) {
      const input = dialog.inputEntity;
      if (input instanceof Api.InputPeerChannel) {
        return input;
      }
    }
  }
  throw new Error(`Private channel ${channelID} not found in dialogs.`);
}

function parseMessageLink(link: string):
  | { kind: "username"; username: string; messageID: number }
  | { kind: "private"; channelID: string; messageID: number } {
  const url = new URL(link);
  const parts = url.pathname.split("/").filter(Boolean);
  if (parts.length < 2) {
    throw new Error(`Unexpected MESSAGE_LINK: ${link}`);
  }

  const messageID = Number.parseInt(parts.at(-1) ?? "", 10);
  if (!Number.isSafeInteger(messageID)) {
    throw new Error(`Invalid message ID in MESSAGE_LINK: ${link}`);
  }

  if (parts[0] === "c") {
    const channelID = parts[1];
    if (!channelID || !/^\d+$/.test(channelID)) {
      throw new Error(`Invalid private channel ID in MESSAGE_LINK: ${link}`);
    }
    return { kind: "private", channelID, messageID };
  }

  const username = parts[0];
  if (!username) {
    throw new Error(`Invalid username in MESSAGE_LINK: ${link}`);
  }
  return { kind: "username", username, messageID };
}

function progressLogger(prefix: string) {
  let next = 50 * 1024 * 1024;
  return (done: { toJSNumber(): number }, total: { toJSNumber(): number }) => {
    const doneBytes = done.toJSNumber();
    const totalBytes = total.toJSNumber();
    if (totalBytes <= 0) {
      return;
    }
    if (doneBytes >= next || doneBytes >= totalBytes) {
      console.log(
        `${prefix} ${doneBytes} / ${totalBytes} bytes (${
          (
            (doneBytes * 100) /
            totalBytes
          ).toFixed(1)
        }%)`,
      );
      while (next <= doneBytes) {
        next += 50 * 1024 * 1024;
      }
    }
  };
}
