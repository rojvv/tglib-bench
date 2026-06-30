import { unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ok } from "node:assert";
import { TelegramClient } from "teleproto";
import { StringSession } from "teleproto/sessions/index.js";
import env from "./env.ts";

function parseMessageLink(link: string): [number | string, number] {
  const { pathname } = new URL(link);
  const [, chatId, messageId] = pathname.replace(/^\/c/, "").split("/");
  const asNum = Number(chatId);
  return [
    Number.isFinite(asNum) ? -1_000_000_000_000 - asNum : chatId,
    Number(messageId),
  ];
}

const client = new TelegramClient(
  new StringSession(env.AUTH_STRING),
  env.API_ID,
  env.API_HASH,
  { connectionRetries: 5 },
);
await client.connect();

const [chat, messageId] = parseMessageLink(env.MESSAGE_LINK);
const [msg] = await client.getMessages(chat, { ids: [messageId] });
ok(msg != null);
ok(msg.document != null);

const fileSize = msg.document.size.toJSNumber();
const dates = new Array<Date>();

const tmpPath = join(tmpdir(), `teleproto-bench-${process.pid}`);

dates.push(new Date());
const downloaded = await client.downloadMedia(msg, { outputFile: tmpPath });
ok(downloaded === tmpPath);
dates.push(new Date());

dates.push(new Date());
await client.sendFile(env.CHAT_ID, { file: tmpPath, forceDocument: true });
dates.push(new Date());

await client.destroy();
unlinkSync(tmpPath);
writeFileSync(
  "results.json",
  JSON.stringify([fileSize, dates.map((v) => v.getTime() / 1_000)]),
);
