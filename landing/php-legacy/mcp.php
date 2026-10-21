<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';

function mcp_mailcow(string $verb, string $path, ?array $body = null): array
{
    $base = getenv('MAILCOW_API_URL');
    $base = is_string($base) && $base !== '' ? rtrim($base, '/') : 'http://nginx';
    $key = getenv('MAILCOW_API_KEY');
    $ch = curl_init($base . $path);
    curl_setopt_array($ch, [
        CURLOPT_CUSTOMREQUEST => $verb,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_CONNECTTIMEOUT => 5,
        CURLOPT_TIMEOUT => 20,
        CURLOPT_HTTPHEADER => [
            'X-API-Key: ' . (is_string($key) ? $key : ''),
            'Content-Type: application/json',
        ],
    ]);
    if ($body !== null) {
        curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($body));
    }
    $res = curl_exec($ch);
    $errno = curl_errno($ch);
    curl_close($ch);
    if (!is_string($res) || $errno !== 0) {
        return ['ok' => false, 'error' => 'mailcow API unreachable at ' . $base];
    }
    return ['ok' => true, 'data' => json_decode($res, true)];
}

function mcp_result(mixed $id, array $result): array
{
    return ['jsonrpc' => '2.0', 'id' => $id, 'result' => $result];
}

function mcp_error(mixed $id, int $code, string $message): array
{
    return ['jsonrpc' => '2.0', 'id' => $id, 'error' => ['code' => $code, 'message' => $message]];
}

function mcp_tools(): array
{
    return [
        [
            'name' => 'list_domains',
            'description' => 'List all mail domains managed by this multi.ai.inbox (mailcow) instance, with quota, alias limits, active and relay state.',
            'inputSchema' => [
                'type' => 'object',
                'properties' => new stdClass(),
                'additionalProperties' => false,
            ],
        ],
        [
            'name' => 'get_domain_settings',
            'description' => 'Get the full settings of a single mail domain: quota, max quota, aliases, mailboxes, DKIM, relay state, active state.',
            'inputSchema' => [
                'type' => 'object',
                'required' => ['domain'],
                'properties' => [
                    'domain' => ['type' => 'string', 'description' => 'Domain name, e.g. example.com'],
                ],
                'additionalProperties' => false,
            ],
        ],
        [
            'name' => 'set_domain_settings',
            'description' => 'Update settings of a single mail domain. Only include the attributes to change, e.g. {"active":"1"} to activate, {"defquota":"3072"} for default quota in MiB, {"backupmx":"1"} to relay, {"rl_value":"10","rl_frame":"s"} for rate limit.',
            'inputSchema' => [
                'type' => 'object',
                'required' => ['domain', 'attrs'],
                'properties' => [
                    'domain' => ['type' => 'string', 'description' => 'Domain name to update, e.g. example.com'],
                    'attrs' => ['type' => 'object', 'description' => 'Attributes to change, as accepted by the mailcow /api/v1/edit/domain attr object'],
                ],
                'additionalProperties' => false,
            ],
        ],
    ];
}

function mcp_tool_call(string $tool, array $args): array
{
    $domain = strtolower(trim((string)($args['domain'] ?? '')));
    if ($domain !== '' && preg_match('/^(?=.{1,253}$)([a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?\.)+([a-z]{2,63}|xn--[a-z0-9-]{4,59})$/', $domain) !== 1) {
        return ['ok' => false, 'error' => 'Invalid domain name: ' . $domain];
    }
    switch ($tool) {
        case 'list_domains':
            return mcp_mailcow('GET', '/api/v1/get/domain/all');
        case 'get_domain_settings':
            if ($domain === '') {
                return ['ok' => false, 'error' => 'get_domain_settings requires a valid domain'];
            }
            return mcp_mailcow('GET', '/api/v1/get/domain/' . rawurlencode($domain));
        case 'set_domain_settings':
            $attrs = is_array($args['attrs'] ?? null) ? $args['attrs'] : null;
            if ($attrs === null || $domain === '') {
                return ['ok' => false, 'error' => 'set_domain_settings requires a valid domain and an attrs object'];
            }
            return mcp_mailcow('POST', '/api/v1/edit/domain', ['items' => [$domain], 'attr' => $attrs]);
    }
    return ['unknown' => true];
}

$expected_key = getenv('MAILCOW_API_KEY');
$provided_key = '';
if (preg_match('/^Bearer\s+(\S+)$/i', (string)($_SERVER['HTTP_AUTHORIZATION'] ?? ''), $m) === 1) {
    $provided_key = trim($m[1]);
}
if (!is_string($expected_key) || $expected_key === '' || !hash_equals($expected_key, $provided_key)) {
    json_err('Unauthorized: send "Authorization: Bearer <MAILCOW_API_KEY>" (read-write key from mailcow.conf)', 401);
}

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
    json_err('POST required, this endpoint speaks JSON-RPC 2.0 (MCP Streamable HTTP)', 405);
}

$payload = json_decode((string)file_get_contents('php://input'), true);
if (!is_array($payload) || ($payload['jsonrpc'] ?? '') !== '2.0' || !is_string($payload['method'] ?? null)) {
    json_err('Invalid JSON-RPC 2.0 request', 400);
}

$method = $payload['method'];
$params = is_array($payload['params'] ?? null) ? $payload['params'] : [];
$id = $payload['id'] ?? null;

if ($method === 'initialize') {
    json_out(mcp_result($id, [
        'protocolVersion' => '2025-06-18',
        'capabilities' => ['tools' => ['listChanged' => false]],
        'serverInfo' => ['name' => 'multi-ai-inbox-mcp', 'title' => 'multi.ai.inbox MCP', 'version' => '1.0.0'],
        'instructions' => 'Tools read and update mail domain settings of this multi.ai.inbox instance via the mailcow REST API.',
    ]));
}
if (str_starts_with($method, 'notifications/')) {
    http_response_code(202);
    exit;
}
if ($method === 'tools/list') {
    json_out(mcp_result($id, ['tools' => mcp_tools()]));
}
if ($method === 'tools/call') {
    $tool = (string)($params['name'] ?? '');
    $args = is_array($params['arguments'] ?? null) ? $params['arguments'] : [];
    $call = mcp_tool_call($tool, $args);
    if ($call['unknown'] ?? false) {
        json_out(mcp_result($id, ['content' => [['type' => 'text', 'text' => 'Unknown tool: ' . $tool]], 'isError' => true]));
    }
    if (!$call['ok']) {
        json_out(mcp_result($id, ['content' => [['type' => 'text', 'text' => $call['error']]], 'isError' => true]));
    }
    json_out(mcp_result($id, ['content' => [['type' => 'text', 'text' => json_encode($call['data'], JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES)]]]));
}
json_out(mcp_error($id, -32601, 'Method not found: ' . $method));
