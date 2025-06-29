import { writeFileSync } from "node:fs";
import { ok } from "node:assert";
import { InputMedia, TelegramClient } from "@mtcute/node";
import env from "./env.ts";

const tg = new TelegramClient({
  apiId: env.API_ID,
  apiHash: env.API_HASH,
});
await tg.importSession(env.AUTH_STRING);
await tg.start();

const msg = await tg.getMessageByLink(env.MESSAGE_LINK);
ok(msg != null);
ok(msg.media != null);
ok(msg.media.type === "document");

const dates = new Array<Date>();
const chunks = new Array<Uint8Array>();

dates.push(new Date());
for await (const chunk of tg.downloadAsIterable(msg.media)) {
  chunks.push(chunk);
}
dates.push(new Date());

const document = new Uint8Array(await new Blob(chunks).arrayBuffer());

dates.push(new Date());
await tg.sendMedia(env.CHAT_ID, InputMedia.document(document));
dates.push(new Date());

await tg.destroy();
writeFileSync(
  "results.json",
  JSON.stringify([document.byteLength, dates.map((v) => v.getTime() / 1_000)]),
);
