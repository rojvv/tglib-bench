import { Client } from "@mtkruto/mtkruto";
import env from "./env.ts";

const client = new Client({ authString: env.AUTH_STRING });

await client.start();

const message = await client.resolveMessageLink(env.MESSAGE_LINK);
if (!message) {
  console.error("Message not found.");
  Deno.exit(1);
}
if (!("document" in message)) {
  console.error("Message invalid.");
  Deno.exit(1);
}

const dates = new Array<Date>();
dates.push(new Date());

const chunks = new Array<Uint8Array>();
for await (const chunk of client.download(message.document.fileId)) {
  chunks.push(chunk);
}
dates.push(new Date());

const document = new Uint8Array(await new Blob(chunks).arrayBuffer());
dates.push(new Date());
await client.sendDocument(env.CHAT_ID, document);
dates.push(new Date());

Deno.writeTextFileSync(
  "results.json",
  JSON.stringify(dates.map((v) => v.getTime() / 1_000)),
);
