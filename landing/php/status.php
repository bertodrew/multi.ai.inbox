<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';

if ($_SERVER['REQUEST_METHOD'] !== 'GET') {
    json_err('GET required', 405);
}

$connected = !empty($_SESSION['nc_connected']);
$creds = $connected ? read_credentials() : null;

json_out([
    'ok' => true,
    'connected' => $connected && $creds !== null,
    'username' => $connected && $creds !== null ? ($_SESSION['nc_username'] ?? $creds['username']) : null,
    'sandbox' => $creds !== null ? (bool)($creds['sandbox'] ?? false) : false,
    'csrf' => ensure_csrf(),
    'mailcow_origin' => mailcow_origin(),
    'public_ip' => detect_public_ip(),
]);
