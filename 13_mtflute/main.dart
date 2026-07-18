import 'dart:convert';
import 'dart:io';

import 'package:mtflute/mtflute.dart';

const String _libVersion = '0.2.8';
const int _threads = 4;

Future<void> main(List<String> args) async {
  final cfg = _loadEnv();

  final client = MtpClient(
    appId: cfg.apiId,
    appHash: cfg.apiHash,
    dcId: 4,
    stringSession: cfg.authString,
    sessionFile: null,
    timeout: const Duration(seconds: 60),
  );
  client.logger.level = LogLevel.warn;

  await client.connect();

  final peerAndId = await _resolvePeer(client, cfg.messageLink);
  final msg = await client.getMessage(peer: peerAndId.$1, id: peerAndId.$2);
  if (msg is! MessageObj) {
    stderr.writeln('Message not found: ${msg?.runtimeType}');
    exit(1);
  }
  final media = msg.media;
  if (media is! MessageMediaDocument) {
    stderr.writeln('Message has no document.');
    exit(1);
  }
  final doc = media.document;
  if (doc is! DocumentObj) {
    stderr.writeln('Message document unavailable.');
    exit(1);
  }

  final loc = InputDocumentFileLocation(
    id: doc.id,
    accessHash: doc.accessHash,
    fileReference: doc.fileReference,
    thumbSize: '',
  );

  final timestamps = <double>[];
  final tmp = File('mtflute.bin');

  timestamps.add(_now());
  final sink = tmp.openWrite();
  try {
    await for (final chunk in client.downloadStream(
      loc,
      dcId: doc.dcId,
      size: doc.size,
      chunkSize: 1024 * 1024,
      threads: _threads,
    )) {
      sink.add(chunk);
    }
  } finally {
    await sink.flush();
    await sink.close();
  }
  timestamps.add(_now());

  timestamps.add(_now());
  final uploaded = await client.uploadFile(
    tmp,
    fileName: 'mtflute.bin',
    threads: _threads,
  );
  timestamps.add(_now());

  final target = await _resolveChatIdPeer(client, cfg.chatId);
  await client.sendMedia(
    peer: target,
    media: InputMediaUploadedDocument(
      file: uploaded,
      mimeType: 'application/octet-stream',
      attributes: [DocumentAttributeFilename(fileName: 'mtflute.bin')],
    ),
  );

  try { await tmp.delete(); } catch (_) {}
  await client.close();

  final result = [doc.size, timestamps, _libVersion];
  await File('results.json').writeAsString(jsonEncode(result));
  exit(0);
}

double _now() => DateTime.now().millisecondsSinceEpoch / 1000.0;

class _Env {
  final int apiId;
  final String apiHash;
  final String authString;
  final String messageLink;
  final int chatId;
  _Env({
    required this.apiId,
    required this.apiHash,
    required this.authString,
    required this.messageLink,
    required this.chatId,
  });
}

_Env _loadEnv() {
  String req(String k) {
    final v = Platform.environment[k];
    if (v == null || v.isEmpty) {
      stderr.writeln('$k not set');
      exit(1);
    }
    return v;
  }

  return _Env(
    apiId: int.parse(req('API_ID')),
    apiHash: req('API_HASH'),
    authString: req('AUTH_STRING'),
    messageLink: req('MESSAGE_LINK'),
    chatId: int.parse(req('CHAT_ID')),
  );
}

Future<(InputPeer, int)> _resolvePeer(MtpClient client, String link) async {
  final uri = Uri.parse(link);
  final parts = uri.pathSegments.where((p) => p.isNotEmpty).toList();
  if (parts.length < 2) {
    stderr.writeln('Unexpected message link: $link');
    exit(1);
  }
  final msgId = int.parse(parts.last);
  final chatPart = parts[parts.length - 2];

  if (parts.first == 'c') {
    final channelId = int.parse(chatPart);
    final ch = await client.getChannel(channelId, 0);
    if (ch is Channel) {
      return (
        InputPeerChannel(channelId: ch.id, accessHash: ch.accessHash ?? 0),
        msgId,
      );
    }
    return (InputPeerChannel(channelId: channelId, accessHash: 0), msgId);
  }

  final r = await client.resolveUsername(chatPart);
  if (r is! ContactsResolvedPeerObj) {
    stderr.writeln('Cannot resolve username $chatPart');
    exit(1);
  }
  final p = r.peer;
  if (p is PeerChannel) {
    for (final c in r.chats) {
      if (c is Channel && c.id == p.channelId) {
        return (
          InputPeerChannel(channelId: c.id, accessHash: c.accessHash ?? 0),
          msgId,
        );
      }
    }
  } else if (p is PeerUser) {
    for (final u in r.users) {
      if (u is UserObj && u.id == p.userId) {
        return (
          InputPeerUser(userId: u.id, accessHash: u.accessHash ?? 0),
          msgId,
        );
      }
    }
  } else if (p is PeerChat) {
    return (InputPeerChat(chatId: p.chatId), msgId);
  }
  stderr.writeln('Unresolvable peer for $chatPart');
  exit(1);
}

Future<InputPeer> _resolveChatIdPeer(MtpClient client, int chatId) async {
  try {
    return client.cache.getInputPeer(chatId);
  } catch (_) {}

  if (chatId < 0) {
    final channelId = chatId < -1000000000000
        ? -chatId - 1000000000000
        : -chatId;
    final ch = await client.getChannel(channelId, 0);
    if (ch is Channel) {
      return InputPeerChannel(
        channelId: ch.id,
        accessHash: ch.accessHash ?? 0,
      );
    }
    stderr.writeln('Cannot resolve chat id $chatId');
    exit(1);
  }
  stderr.writeln(
      'User chat id $chatId is not in cache; run resolveUsername first.');
  exit(1);
}
