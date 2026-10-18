<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';
require_once __DIR__ . '/namecheap.php';

if ($_SERVER['REQUEST_METHOD'] !== 'GET') {
    json_err('GET required', 405);
}
$creds = require_namecheap_session();

$domain = strtolower(trim((string)($_GET['domain'] ?? '')));
if (preg_match('/^(?=.{1,253}$)([a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?\.)+([a-z]{2,63}|xn--[a-z0-9-]{4,59})$/', $domain) !== 1) {
    json_err('Invalid domain name');
}

$res = nc_dns_hosts($creds, $domain);
if (!$res['ok']) {
    json_err($res['error'], 502);
}
json_out(['ok' => true, 'domain' => $domain, 'hosts' => $res['hosts']]);
