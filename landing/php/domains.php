<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';
require_once __DIR__ . '/namecheap.php';

if ($_SERVER['REQUEST_METHOD'] !== 'GET') {
    json_err('GET required', 405);
}
$creds = require_namecheap_session();

$res = nc_domains($creds);
if (!$res['ok']) {
    json_err($res['error'], 502);
}
json_out(['ok' => true, 'domains' => $res['domains']]);
