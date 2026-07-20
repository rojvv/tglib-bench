<?php declare(strict_types=1);

/**
 * This is a single file test of the LaraGram MTProto library.
 * For production use it must be used under the LaraGram framework,
 * @see {[LaraGram MTProto](https://laraxgram.github.io/v4/mtproto.html)} for documentation.
 */


require __DIR__ . '/vendor/autoload.php';

use LaraGram\MTProto\Auth\Authorization;
use LaraGram\MTProto\Core\Client;
use LaraGram\MTProto\Crypto\FfiIgeCrypto;
use LaraGram\MTProto\Foundation\FileUploader;
use LaraGram\MTProto\Foundation\InputMedia;

if (!extension_loaded('swoole') && !extension_loaded('openswoole')) {
    throw new RuntimeException('ext-swoole (or openswoole) is required for the parallel benchmark.');
}

define('UPLOAD_CONCURRENCY', (int) (getenv('UPLOAD_CONCURRENCY') ?: 16));

define('DOWNLOAD_CONCURRENCY', (int) (getenv('DOWNLOAD_CONCURRENCY') ?: 32));

define('MEDIA_SOCKETS', (int) (getenv('MEDIA_SOCKETS') ?: 8));

function requiredEnv(string $name): string {
    $value = getenv($name);
    if ($value === false || $value === '') {
        throw new RuntimeException("$name not set");
    }
    return $value;
}

function parsePositiveInt(string $value, string $name): int {
    if (!ctype_digit($value) || (int) $value === 0) {
        throw new RuntimeException("Invalid $name");
    }
    return (int) $value;
}

function requiredIntEnv(string $name): int {
    return parsePositiveInt(requiredEnv($name), $name);
}

function requiredChannelPeerEnv(string $name): int {
    $value = requiredEnv($name);
    if (!preg_match('/^-100\d+$/', $value)) {
        throw new RuntimeException("Invalid $name");
    }
    return (int) $value;
}

$EXPORT_AUTH_STRING = (bool) getenv('EXPORT_AUTH_STRING');
$AUTH_STRING = getenv('AUTH_STRING') ?: null;

$API_ID = requiredIntEnv("API_ID");
$API_HASH = requiredEnv("API_HASH");
$BOT_TOKEN = getenv('BOT_TOKEN') ?: null;

if ($EXPORT_AUTH_STRING) {
    if ($BOT_TOKEN === null) {
        throw new RuntimeException('BOT_TOKEN not set (required to mint a new AUTH_STRING)');
    }
} elseif ($AUTH_STRING === null && $BOT_TOKEN === null) {
    throw new RuntimeException('Set AUTH_STRING (preferred) or BOT_TOKEN');
}

$messageLink = $EXPORT_AUTH_STRING ? '' : requiredEnv("MESSAGE_LINK");
$CHAT_ID = $EXPORT_AUTH_STRING ? 0 : requiredChannelPeerEnv("CHAT_ID");

$client = new Client($API_ID, $API_HASH, [
    'session_dir'       => __DIR__ . '/sessions',
    'flood_sleep_limit' => 60 * 60,
    'use_pump'          => true,
    'transfer'          => ['media_sockets' => MEDIA_SOCKETS],
    'auth_string'       => $EXPORT_AUTH_STRING ? null : $AUTH_STRING,
]);

printf(
    "crypto: %s (ffi.enable=%s)  sockets=%d  up_window=%d  dl_window=%d\n",
    get_class($client->getCrypto()),
    (string) ini_get('ffi.enable'),
    MEDIA_SOCKETS,
    UPLOAD_CONCURRENCY,
    DOWNLOAD_CONCURRENCY,
);
if (!FfiIgeCrypto::isSupported()) {
    fwrite(STDERR, "WARNING: FFI AES-IGE unavailable - downloads will be CPU-bound on the slow per-block decrypt path!\n");
}

$client->connect('session.mtproto');

if ($client->getSession()->getAuthKey() === null) {
    if ($BOT_TOKEN === null) {
        throw new RuntimeException('AUTH_STRING did not yield a usable session and no BOT_TOKEN was given to log in with.');
    }
    (new Authorization($client))->botLogin($BOT_TOKEN);
}

if ($EXPORT_AUTH_STRING) {
    fwrite(STDERR, "Session authenticated. AUTH_STRING (store as a secret):\n");
    echo $client->authString(), "\n";
    $client->disconnect();
    exit(0);
}

function getMessageDetails(string $messageLink): array {
    $path = parse_url($messageLink, PHP_URL_PATH);
    if (!is_string($path)) {
        throw new Exception("Invalid message link format.");
    }

    $parts = explode('/', trim($path, '/'));
    if (count($parts) < 2) {
        throw new Exception("Invalid message link format.");
    }

    $messageId = parsePositiveInt($parts[count($parts) - 1], "message id");
    $chatPart = $parts[count($parts) - 2];

    if ($parts[0] === 'c') {
        $channelId = parsePositiveInt($chatPart, "channel id");
        return [-1000000000000 - $channelId, $messageId];
    }

    if (!preg_match('/^[A-Za-z0-9_]+$/', $chatPart)) {
        throw new Exception("Invalid message link format.");
    }
    $chatId = "@" . $chatPart;

    return [$chatId, $messageId];
}

function downloadFile(Client $client, string|int $chatId, int $messageId): array {
    $message = $client->getMessages($chatId, $messageId)['messages'][0] ?? null;
    $media = $message['media'] ?? null;
    if ($media === null || ($media['_'] ?? 'messageMediaEmpty') === 'messageMediaEmpty') {
        throw new AssertionError("No media found in request");
    }

    $fileSize = $client->getFileInfo($media)['size'] ?? 0;

    echo "Starting warm-up download...\n";
    $warmupFile = tempnam('/tmp', 'mtp_');
    $client->downloadMediaToFile($media, $warmupFile);
    if (!is_file($warmupFile) || !unlink($warmupFile)) {
        throw new RuntimeException("Failed to remove warm-up download: $warmupFile");
    }
    echo "Warm-up download completed.\n";

    $file = tempnam('/tmp', 'mtp_');
    $startTime = microtime(true);
    $written = $client->downloadMediaToFile($media, $file);
    $endTime = microtime(true);
    if ($fileSize > 0 && $written !== $fileSize) {
        throw new RuntimeException("Incomplete download: {$written} of {$fileSize} bytes");
    }
    echo "Download completed in " . ($endTime - $startTime) . " seconds.\n";

    return [
        "file" => $file,
        "timestamps" => [$startTime, $endTime],
        'file_size' => $fileSize,
    ];
}

function uploadFile(Client $client, string|int $chatId, string $filePath): array {
    $startTime = microtime(true);
    $lastProgressLog = 0.0;
    echo "Starting upload...\n";

    $inputFile = (new FileUploader($client))
        ->withConcurrency(UPLOAD_CONCURRENCY)
        ->fromPath(
            $filePath,
            "MTProto.zip",
            static function (int $partsDone, int $partsTotal) use (&$lastProgressLog, $startTime): void {
                $percent = $partsTotal > 0 ? $partsDone / $partsTotal * 100 : 0;
                $time = microtime(true) - $startTime;
                if ($percent >= 100 || $time - $lastProgressLog >= 10) {
                    $lastProgressLog = $time;
                    echo sprintf(
                        "Upload progress %.2f%% after %.2f seconds.\n",
                        $percent,
                        $time
                    );
                }
            }
        );

    $client->sendMedia(
        $chatId,
        InputMedia::uploadedDocument(
            $inputFile,
            'application/zip',
            [InputMedia::attrFilename('MTProto.zip')],
            forceFile: true,
        ),
        'Powered by laraxgram/mtproto!',
    );

    $endTime = microtime(true);
    unlink($filePath);
    echo "Upload completed in " . ($endTime - $startTime) . " seconds.\n";
    return [$startTime, $endTime];
}

$client->getConnection()->disconnect();

$results = null;

$client->getRuntime()->run(function () use ($client, $messageLink, $CHAT_ID, &$results): void {
    $client->reconnect();
    $client->startPump();

    $client->fileDecoder()->withConcurrency(DOWNLOAD_CONCURRENCY);

    list($chatId, $messageId) = getMessageDetails($messageLink);

    $fileMI = downloadFile($client, $chatId, $messageId);
    $filePath = $fileMI["file"];
    unset($fileMI["file"]);

    $client->primePeerCache();
    $uploadTimestamps = uploadFile($client, $CHAT_ID, $filePath);

    $results = [
        $fileMI['file_size'],
        [
            $fileMI['timestamps'][0],
            $fileMI['timestamps'][1],
            $uploadTimestamps[0],
            $uploadTimestamps[1],
        ],
        Client::VERSION,
    ];

    $dumpStats = static function (string $label, Client $c): void {
        if (!$c->supportsConcurrentInvoke()) {
            return;
        }
        $s = $c->getPump()->getStats();
        fprintf(
            STDERR,
            "stats %s: in %d/%.1fMB out %d/%.1fMB rpc_err %d timeouts %d resends %d reconnects %d transport_err %d garbage %d\n",
            $label,
            $s['frames_in'], $s['bytes_in'] / 1048576,
            $s['frames_out'], $s['bytes_out'] / 1048576,
            $s['rpc_errors'], $s['timeouts'], $s['resends'], $s['reconnects'],
            $s['transport_errors'], $s['garbage_frames'],
        );
    };
    $dumpStats('home', $client);
    for ($dc = 1; $dc <= 5; $dc++) {
        $live = $client->mediaPool()->countFor($dc);
        if ($live > 0) {
            foreach ($client->mediaSockets($dc, $live) as $i => $s) {
                if ($s !== $client) {
                    $dumpStats("dc{$dc}#{$i}", $s);
                }
            }
        }
    }

    if ($results !== null) {
        file_put_contents("results.json", json_encode($results));

        $mb = $results[0] / (1024 * 1024);
        $dlSecs = $results[1][1] - $results[1][0];
        $upSecs = $results[1][3] - $results[1][2];
        printf(
            "\n=== Throughput (%.2f MB) ===\nDownload: %.2f MB/s (%.2fs)\nUpload:   %.2f MB/s (%.2fs)\n",
            $mb,
            $dlSecs > 0 ? $mb / $dlSecs : 0,
            $dlSecs,
            $upSecs > 0 ? $mb / $upSecs : 0,
            $upSecs,
        );
    }

    $client->disconnect();
});
