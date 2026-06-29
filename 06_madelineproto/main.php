<?php declare(strict_types=1);

// From https://github.com/TelegramPlayground/bmt/tree/master/src/madelineproto

/**
 * Example bot. https://t.me/TrollVoiceBot?start=1266
 *
 * Copyright 2016-2020 Daniil Gentili
 * (https://daniil.it)
 * This file is part of MadelineProto.
 * MadelineProto is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.
 * MadelineProto is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
 * See the GNU Affero General Public License for more details.
 * You should have received a copy of the GNU General Public License along with MadelineProto.
 * If not, see <http://www.gnu.org/licenses/>.
 *
 * @author    Daniil Gentili <daniil@daniil.it>
 * @copyright 2016-2023 Daniil Gentili <daniil@daniil.it>
 * @license   https://opensource.org/licenses/AGPL-3.0 AGPLv3
 * @link https://docs.madelineproto.xyz MadelineProto documentation
 */

/*
 * Various ways to load @MadeLineProto
 */
if (file_exists(__DIR__ . "/vendor/autoload.php")) {
    include __DIR__ . "/vendor/autoload.php";
} else {
    if (!file_exists("madeline.php")) {
        copy("https://phar.madelineproto.xyz/madeline.php", "madeline.php");
    }
    /**
     * @psalm-suppress MissingFile
     */
    include "madeline.php";
}

use danog\MadelineProto\API;
use danog\MadelineProto\EventHandler\Message;
use danog\MadelineProto\LocalFile;
use Webmozart\Assert\Assert;

Assert::true(extension_loaded('uv'), "The uv extension is required for maximum performance");
Assert::true(opcache_get_status(false)['jit']['enabled'], "JIT is required for maximum performance");

/**
 * required environment variables
 */

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

$API_ID = requiredIntEnv("API_ID");
$API_HASH = requiredEnv("API_HASH");
$BOT_TOKEN = requiredEnv("BOT_TOKEN");
$messageLink = requiredEnv("MESSAGE_LINK");
$CHAT_ID = requiredChannelPeerEnv("CHAT_ID");

/**
 * @MadeLineProto Settings
 */

$settings = new \danog\MadelineProto\Settings;
$settings->getLogger()->setLevel(\danog\MadelineProto\Logger::LEVEL_ULTRA_VERBOSE);

$settings->getConnection()->setMaxMediaSocketCount(50);
$settings->getRpc()->setRpcDropTimeout(60 * 60);
$settings->getFiles()->setUploadParallelChunks(50);
$settings->getFiles()->setDownloadParallelChunks(50);
// IMPORTANT: for security reasons, upload by URL will still be allowed
$settings->getFiles()->setAllowAutomaticUpload(true);

$settings->getAppInfo()
    ->setApiId($API_ID)
    ->setApiHash($API_HASH);

$api = new \danog\MadelineProto\API('session.madeline', $settings);
$api->botLogin($BOT_TOKEN);
$api->start();
$api->fullGetSelf();

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

function downloadFile(API $api, string|int $chatId, int $messageId): array {
    $message = $api->wrapMessage($api->channels->getMessages([
        'channel' => $chatId,
        'id' => [$messageId]
    ])['messages'][0]);
    if (!$message instanceof Message || $message->media === null) {
        throw new AssertionError("No media found in request");
    }

    $startTime = microtime(true);
    $file = $message->media->downloadToDir('/tmp');
    $endTime = microtime(true);
    echo "Download completed in " . ($endTime - $startTime) . " seconds.\n";

    return [
        "file" => $file,
        "timestamps" => [$startTime, $endTime],
        'file_size' => $message->media->size,
    ];
}

function uploadFile(API $api, string|int $chatId, string $filePath): array {
    $startTime = microtime(true);
    $api->sendDocument(
        peer: $chatId,
        file: new LocalFile($filePath),
        fileName: "MadelineProto.zip",
        caption: 'Powered by @MadelineProto!'
    );
    $endTime = microtime(true);
    unlink($filePath);
    echo "Upload completed in " . ($endTime - $startTime) . " seconds.\n";
    return [$startTime, $endTime];
}

function resolveUploadPeer(API $api, int $chatId): int {
    if (!$api->peerIsset($chatId)) {
        $api->getDialogIds();
    }
    if (!$api->peerIsset($chatId)) {
        throw new RuntimeException(
            "CHAT_ID is not present in MadelineProto's peer database; make sure the bot is a member of that chat and has received an update from it, or use a cached session that already knows the peer."
        );
    }
    return $chatId;
}

list($chatId, $messageId) = getMessageDetails($messageLink);
$fileMI = downloadFile($api, $chatId, $messageId);
$filePath = $fileMI["file"];
unset($fileMI["file"]);
$uploadTimestamps = uploadFile($api, resolveUploadPeer($api, $CHAT_ID), $filePath);

$j = [
    $fileMI['file_size'],
    [
        $fileMI['timestamps'][0],
        $fileMI['timestamps'][1],
        $uploadTimestamps[0],
        $uploadTimestamps[1],
    ],
    API::RELEASE,
];
file_put_contents("results.json", json_encode($j));
?>
