using System.Reflection;
using System.Text.Json;
using TL;

// Expected env vars: api_id, api_hash, bot_token, message_link, chat_id
// NOTE: For reliable measurements, the session file WTelegram.session should be conserved between runs.

using var client = new WTelegram.Client(Environment.GetEnvironmentVariable);
await client.LoginBotIfNeeded();

var chatId = -1000000000000 - long.Parse(Environment.GetEnvironmentVariable("chat_id")!);
var mc = await client.Channels_GetChannels(new InputChannel(chatId, 0));
var chat = mc.chats[chatId];

var mcm = await client.GetMessageByLink(Environment.GetEnvironmentVariable("message_link"));
if (mcm.messages[0] is not Message { media: MessageMediaDocument { document: Document document } } msg)
	return 1; // failed to find document to download
WTelegram.Helpers.Log = (l, s) => { };
using var stream = new MemoryStream((int)document.size);

Console.WriteLine($"Downloading {document}, {document.size:N0} bytes on DC {document.dc_id} (my DC: {client.TLConfig.this_dc})");
var dstart = DateTimeOffset.UtcNow;
await client.DownloadFileAsync(document, stream);
var dend = DateTimeOffset.UtcNow;
Console.WriteLine($"Downloaded in {dend - dstart}, speed {document.size / (dend - dstart).TotalSeconds / 1024 / 1024:N2} MB/s");

stream.Position = 0;
await Task.Delay(1000);
GC.Collect();

Console.WriteLine($"Uploading document, {stream.Length:N0} bytes");
var ustart = DateTimeOffset.UtcNow;
var uploadedFile = await client.UploadFileAsync(stream, "WTelegramClient.bin");
_ = client.SendMediaAsync(chat, "", uploadedFile);
var uend = DateTimeOffset.UtcNow;
Console.WriteLine($"Upload in {uend - ustart}, speed {document.size / (uend - ustart).TotalSeconds / 1024 / 1024:N2} MB/s");


var data = new object[] {
	document.size,
	new[]
	{
		dstart.ToUnixTimeMilliseconds() / 1000.0,
		dend.ToUnixTimeMilliseconds() / 1000.0,
		ustart.ToUnixTimeMilliseconds() / 1000.0,
		uend.ToUnixTimeMilliseconds() / 1000.0,
	},
	typeof(WTelegram.Client).Assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()!.InformationalVersion?.Split('+')[0]!
};
File.WriteAllText("results.json", JsonSerializer.Serialize(data, new JsonSerializerOptions { WriteIndented = true }));

Console.WriteLine($"Results: Download {document.size / (dend - dstart).TotalSeconds / 1024 / 1024:N2} MB/s | Upload {document.size / (uend - ustart).TotalSeconds / 1024 / 1024:N2} MB/s");
return 0;
