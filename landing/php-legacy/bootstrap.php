<?php
declare(strict_types=1);
ini_set('display_errors', '0');

session_name('MULTIAI_LANDING');
session_set_cookie_params([
    'lifetime' => 0,
    'path' => '/',
    'domain' => '',
    'secure' => (($_SERVER['HTTPS'] ?? '') === 'on') || (($_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '') === 'https'),
    'httponly' => true,
    'samesite' => 'Lax',
]);
session_start();

define('CRED_DIR', getenv('LANDING_CRED_DIR') ?: '/credentials');
define('CRED_FILE', CRED_DIR . '/namecheap.json');
define('NC_MAX_ATTEMPTS', 5);
define('NC_ATTEMPT_WINDOW', 600);

function json_out(array $data, int $code = 200): void
{
    http_response_code($code);
    header('Content-Type: application/json; charset=utf-8');
    header('X-Content-Type-Options: nosniff');
    header('Cache-Control: no-store');
    echo json_encode($data, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
    exit;
}

function json_err(string $message, int $code = 400): void
{
    json_out(['ok' => false, 'error' => $message], $code);
}

function ensure_csrf(): string
{
    if (empty($_SESSION['csrf']) || !is_string($_SESSION['csrf'])) {
        $_SESSION['csrf'] = bin2hex(random_bytes(16));
    }
    return $_SESSION['csrf'];
}

function require_csrf(): void
{
    $sent = $_SERVER['HTTP_X_LANDING_CSRF'] ?? ($_POST['csrf'] ?? '');
    if (!is_string($sent) || empty($_SESSION['csrf']) || !hash_equals($_SESSION['csrf'], $sent)) {
        json_err('Invalid CSRF token, reload the page', 403);
    }
}

function store_credentials(array $creds): void
{
    if (!is_dir(CRED_DIR) && !@mkdir(CRED_DIR, 0700, true) && !is_dir(CRED_DIR)) {
        json_err('Credential storage unavailable', 500);
    }
    @chmod(CRED_DIR, 0700);
    $tmp = CRED_DIR . '/.namecheap.' . bin2hex(random_bytes(6));
    if (file_put_contents($tmp, json_encode($creds), LOCK_EX) === false) {
        json_err('Credential storage unavailable', 500);
    }
    @chmod($tmp, 0600);
    @rename($tmp, CRED_FILE);
}

function read_credentials(): ?array
{
    if (!is_file(CRED_FILE)) {
        return null;
    }
    $data = json_decode((string)file_get_contents(CRED_FILE), true);
    if (!is_array($data) || !isset($data['username'], $data['api_key'])) {
        return null;
    }
    return $data;
}

function delete_credentials(): void
{
    if (is_file(CRED_FILE)) {
        @unlink(CRED_FILE);
    }
}

function require_namecheap_session(): array
{
    if (empty($_SESSION['nc_connected'])) {
        json_err('Not connected, connect a Namecheap API key first', 401);
    }
    $creds = read_credentials();
    if ($creds === null) {
        unset($_SESSION['nc_connected'], $_SESSION['nc_username']);
        json_err('Stored connection is gone, connect again', 401);
    }
    return $creds;
}

function register_failed_attempt(): void
{
    $now = time();
    $attempts = is_array($_SESSION['nc_attempts'] ?? null) ? $_SESSION['nc_attempts'] : [];
    $attempts = array_values(array_filter($attempts, static fn($t): bool => is_int($t) && ($now - $t) < NC_ATTEMPT_WINDOW));
    $attempts[] = $now;
    $_SESSION['nc_attempts'] = $attempts;
    if (count($attempts) > NC_MAX_ATTEMPTS) {
        json_err('Too many failed attempts, wait 10 minutes and retry', 429);
    }
}

function clear_failed_attempts(): void
{
    unset($_SESSION['nc_attempts']);
}

function detect_public_ip(): ?string
{
    $cache = CRED_DIR . '/.publicip';
    if (is_file($cache)) {
        $ip = trim((string)file_get_contents($cache));
        if (filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE) !== false) {
            return $ip;
        }
    }
    $ch = curl_init('https://api.ipify.org');
    curl_setopt_array($ch, [
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_CONNECTTIMEOUT => 3,
        CURLOPT_TIMEOUT => 5,
    ]);
    $ip = curl_exec($ch);
    curl_close($ch);
    if (!is_string($ip) || filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE) === false) {
        return null;
    }
    if (is_dir(CRED_DIR)) {
        @file_put_contents($cache, $ip . "\n", LOCK_EX);
    }
    return $ip;
}

function mailcow_origin(): string
{
    $env = getenv('MAILCOW_UI_ORIGIN');
    if (is_string($env) && preg_match('#^https?://#i', $env) === 1) {
        return rtrim($env, '/');
    }
    $host = preg_replace('/:\d+$/', '', (string)($_SERVER['HTTP_HOST'] ?? ''));
    return $host !== '' ? 'https://' . $host : '';
}
