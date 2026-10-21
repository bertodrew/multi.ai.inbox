<?php
declare(strict_types=1);
require_once __DIR__ . '/bootstrap.php';

function nc_api_url(bool $sandbox): string
{
    $env = getenv($sandbox ? 'NAMECHEAP_SANDBOX_API_URL' : 'NAMECHEAP_API_URL');
    return is_string($env) && $env !== '' ? $env : ($sandbox
        ? 'https://api.sandbox.namecheap.com/xml.response'
        : 'https://api.namecheap.com/xml.response');
}

function nc_call(array $creds, string $command, array $extra = []): array
{
    $params = array_merge([
        'ApiUser' => $creds['username'],
        'ApiKey' => $creds['api_key'],
        'UserName' => $creds['username'],
        'Command' => $command,
        'ClientIp' => $creds['client_ip'],
    ], $extra);

    $ch = curl_init(nc_api_url((bool)($creds['sandbox'] ?? false)));
    curl_setopt_array($ch, [
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => http_build_query($params),
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_CONNECTTIMEOUT => 10,
        CURLOPT_TIMEOUT => 25,
    ]);
    $body = curl_exec($ch);
    $errno = curl_errno($ch);
    $http = (int)curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
    curl_close($ch);

    if (!is_string($body) || $errno !== 0) {
        return ['ok' => false, 'error' => 'Namecheap API unreachable, check server network', 'http' => 0];
    }
    $xml = @simplexml_load_string($body);
    if ($xml === false) {
        return ['ok' => false, 'error' => 'Invalid Namecheap API response', 'http' => $http];
    }
    if (isset($xml->Errors->Error)) {
        $node = $xml->Errors->Error[0];
        $number = (string)($node['Number'] ?? '');
        $message = trim((string)$node);
        if ($number === '1011031' || stripos($message, 'not whitelisted') !== false) {
            $ip = $creds['client_ip'] !== '' ? $creds['client_ip'] : 'unknown';
            $message = 'IP not whitelisted in Namecheap: add ' . $ip . ' under Profile > Tools > API Access';
        }
        return ['ok' => false, 'error' => $message, 'nc_error' => $number, 'http' => $http];
    }
    return ['ok' => true, 'xml' => $xml];
}

function nc_domains(array $creds): array
{
    $domains = [];
    for ($page = 1; $page <= 10; $page++) {
        $res = nc_call($creds, 'namecheap.domains.getList', ['Page' => $page, 'PageSize' => 100]);
        if (!$res['ok']) {
            return $res;
        }
        $nodes = $res['xml']->xpath('//CommandResponse/DomainGetListResult/Domain') ?: [];
        foreach ($nodes as $node) {
            $domains[] = [
                'name' => (string)($node['Name'] ?? ''),
                'created' => (string)($node['Created'] ?? ''),
                'expires' => (string)($node['Expires'] ?? ''),
                'expired' => ((string)($node['IsExpired'] ?? 'false')) === 'true',
                'locked' => ((string)($node['IsLocked'] ?? 'false')) === 'true',
                'autorenew' => ((string)($node['AutoRenew'] ?? 'false')) === 'true',
                'whoisguard' => (string)($node['WhoisGuard'] ?? 'NOTFOUND'),
            ];
        }
        if (count($nodes) < 100) {
            break;
        }
    }
    return ['ok' => true, 'domains' => $domains];
}

function nc_dns_hosts(array $creds, string $domain): array
{
    $res = nc_call($creds, 'namecheap.domains.dns.getHosts', ['DomainName' => $domain]);
    if (!$res['ok']) {
        return $res;
    }
    $hosts = [];
    $nodes = $res['xml']->xpath('//CommandResponse/DomainDNSGetHostsResult/host') ?: [];
    foreach ($nodes as $node) {
        $hosts[] = [
            'id' => (string)($node['HostID'] ?? ''),
            'name' => (string)($node['Name'] ?? ''),
            'type' => (string)($node['Type'] ?? ''),
            'address' => (string)($node['Address'] ?? ''),
            'mx_pref' => (string)($node['MXPref'] ?? '10'),
            'ttl' => (string)($node['TTL'] ?? '1800'),
        ];
    }
    return ['ok' => true, 'hosts' => $hosts];
}
