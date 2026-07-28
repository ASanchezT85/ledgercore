<?php

declare(strict_types=1);

namespace LedgerCore\Transport;

use LedgerCore\Exception\ConnectionException;

/** Default transport: native cURL, no dependencies. */
final class CurlTransport implements TransportInterface
{
    public function __construct(private readonly int $timeoutSeconds = 10)
    {
    }

    public function request(string $method, string $url, array $headers, ?string $body): TransportResponse
    {
        $headerLines = [];
        foreach ($headers as $name => $value) {
            $headerLines[] = "{$name}: {$value}";
        }

        $responseHeaders = [];
        $ch = curl_init($url);
        if ($ch === false) {
            throw new ConnectionException("could not initialize cURL for {$url}");
        }
        curl_setopt_array($ch, [
            CURLOPT_CUSTOMREQUEST => $method,
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_HTTPHEADER => $headerLines,
            CURLOPT_TIMEOUT => $this->timeoutSeconds,
            CURLOPT_HEADERFUNCTION => static function ($ch, string $line) use (&$responseHeaders): int {
                $parts = explode(':', $line, 2);
                if (count($parts) === 2) {
                    $responseHeaders[strtolower(trim($parts[0]))] = trim($parts[1]);
                }

                return strlen($line);
            },
        ]);
        if ($body !== null) {
            curl_setopt($ch, CURLOPT_POSTFIELDS, $body);
        }

        $responseBody = curl_exec($ch);
        if ($responseBody === false) {
            $error = curl_error($ch);
            curl_close($ch);
            throw new ConnectionException("could not reach LedgerCore: {$error}");
        }
        $status = (int) curl_getinfo($ch, CURLINFO_RESPONSE_CODE);
        curl_close($ch);

        return new TransportResponse($status, (string) $responseBody, $responseHeaders);
    }
}
