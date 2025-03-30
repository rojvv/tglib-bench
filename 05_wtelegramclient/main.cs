using System.Reflection;
using System.Text.Json;
using TL;

// Expected env vars: API_ID, API_HASH, BOT_TOKEN, MESSAGE_LINK, CHAT_ID
// NOTE: For reliable measurements, the session file WTelegram.session should be conserved between runs.

using var client = new WTelegram.Client(Environment.GetEnvironmentVariable);
await client.LoginBotIfNeeded();
var chatId = -1000000000000 - long.Parse(Environment.GetEnvironmentVariable("CHAT_ID")!);
var mc = await client.Channels_GetChannels(new InputChannel(chatId, 0));
var chat = mc.chats[chatId];
var mcm = await client.GetMessageByLink(Environment.GetEnvironmentVariable("MESSAGE_LINK"));
if (mcm.messages[0] is not Message { media: MessageMediaDocument { document: Document document } } msg)
	return 1; // failed to find document to download
WTelegram.Helpers.Log = (l, s) => { };
var ms = new MemoryStream((int)document.size);
var dates = new List<DateTimeOffset>();

dates.Add(DateTimeOffset.UtcNow);
await client.DownloadFileAsync(document, ms);
dates.Add(DateTimeOffset.UtcNow);

ms.Position = 0;

dates.Add(DateTimeOffset.UtcNow);
var uploadedFile = await client.UploadFileAsync(ms, "WTelegramClient.bin");
_ = client.SendMediaAsync(chat, "", uploadedFile, reply_to_msg_id: msg.id);
dates.Add(DateTimeOffset.UtcNow);

var version = typeof(WTelegram.Client).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion?.Split('+')[0];
var data = new object[] {
  document.size,
  new object[]
  {
   dates[0].ToUnixTimeMilliseconds() / 1000.0,
   dates[1].ToUnixTimeMilliseconds() / 1000.0,
   dates[2].ToUnixTimeMilliseconds() / 1000.0,
   dates[3].ToUnixTimeMilliseconds() / 1000.0,
  },
  version,
};
File.WriteAllText("results.json", JsonSerializer.Serialize(data, new JsonSerializerOptions { WriteIndented = true }));
return 0;
