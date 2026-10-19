<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';
require_once __DIR__ . '/namecheap.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    json_err('POST required', 405);
}
require_csrf();

$username = trim((string)($_POST['username'] ?? ''));
$api_key = (string)($_POST['api_key'] ?? '');
$client_ip = trim((string)($_POST['client_ip'] ?? ''));
$sandbox = (string)($_POST['use_sandbox'] ?? '') === '1';

if (!preg_match('/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/', $username)) {
    json_err('Invalid Namecheap username');
}
if (!preg_match('/^[A-Za-z0-9-]{8,128}$/', $api_key)) {
    json_err('Invalid API key format');
}
if ($client_ip !== '' && filter_var($client_ip, FILTER_VALIDATE_IP) === false) {
    json_err('Invalid client IP');
}
if ($client_ip === '') {
    $client_ip = detect_public_ip() ?? '';
}

register_failed_attempt();

$res = nc_call([
    'username' => $username,
    'api_key' => $api_key,
    'client_ip' => $client_ip,
    'sandbox' => $sandbox,
], 'namecheap.domains.getList', ['Page' => 1, 'PageSize' => 5]);

if (!$res['ok']) {
    json_err($res['error'], 401);
}

clear_failed_attempts();
store_credentials([
    'username' => $username,
    'api_key' => $api_key,
    'client_ip' => $client_ip,
    'sandbox' => $sandbox,
]);
$_SESSION['nc_connected'] = true;
$_SESSION['nc_username'] = $username;

json_out(['ok' => true, 'username' => $username, 'sandbox' => $sandbox]);