<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    json_err('POST required', 405);
}
require_csrf();

delete_credentials();
unset($_SESSION['nc_connected'], $_SESSION['nc_username'], $_SESSION['nc_attempts']);

json_out(['ok' => true]);
