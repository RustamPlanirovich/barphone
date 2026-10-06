// HTTP part of the protocol: machine info probing and pairing, plain or TLS pinned to
// the agent's certificate fingerprint (docs/protocol.md, «Шифрование»). Pure Dart.
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';

import 'models.dart';

const _probeTimeout = Duration(milliseconds: 1500);

/// What phones pin: base64url (no padding) of SHA-256 over the certificate (DER).
String certFingerprint(List<int> der) => base64Url.encode(sha256.convert(der).bytes).replaceAll('=', '');

/// TLS to one agent: its certificate must have this fingerprint. [mismatch] tells a
/// certificate that came but was not this one (another or a replaced agent) from a
/// network failure.
class Pin {
  Pin(this.fp);
  final String fp;
  bool mismatch = false;
}

/// An HTTP client; with [pin], TLS that trusts no CA, only the pinned certificate.
HttpClient agentHttpClient(Duration timeout, {Pin? pin}) {
  final c = pin == null ? HttpClient() : HttpClient(context: SecurityContext(withTrustedRoots: false));
  c
    ..connectionTimeout = timeout
    ..userAgent = 'barphone-android/1';
  if (pin != null) {
    c.badCertificateCallback = (cert, host, port) {
      final ok = certFingerprint(cert.der) == pin.fp;
      if (!ok) pin.mismatch = true;
      return ok;
    };
  }
  return c;
}

HttpClient _httpClient(Duration timeout, [Pin? pin]) => agentHttpClient(timeout, pin: pin);

Uri agentUri(String host, int port, String path, {bool secure = false}) =>
    Uri(scheme: secure ? 'https' : 'http', host: host, port: port, path: path);

/// For pictures loaded by Flutter itself (icons over HTTPS): any certificate one of the
/// paired computers is pinned to. Requests to the agents themselves pin exactly.
class PinningOverrides extends HttpOverrides {
  PinningOverrides(this.fingerprints);
  final Set<String> Function() fingerprints;

  @override
  HttpClient createHttpClient(SecurityContext? context) =>
      super.createHttpClient(context)..badCertificateCallback = (cert, host, port) => fingerprints().contains(certFingerprint(cert.der));
}

Future<MachineInfo> fetchInfo(String host, int port, {Duration timeout = _probeTimeout, Pin? pin}) async {
  final client = _httpClient(timeout, pin);
  try {
    final req = await client.getUrl(agentUri(host, port, '/api/v1/info', secure: pin != null)).timeout(timeout);
    final res = await req.close().timeout(timeout);
    final body = await res.transform(utf8.decoder).join().timeout(timeout);
    if (res.statusCode != 200) throw HttpException('info: HTTP ${res.statusCode}');
    return MachineInfo.fromJson((jsonDecode(body) as Map).cast<String, dynamic>());
  } finally {
    client.close(force: true);
  }
}

/// Asks every host in parallel and returns the first one that answers as [expectId]
/// (or as any agent when [expectId] is null). Null if nobody answered.
Future<(String, MachineInfo)?> probeHosts(
  List<String> hosts,
  int port, {
  String? expectId,
  Duration timeout = _probeTimeout,
  Pin? pin,
}) async {
  if (hosts.isEmpty) return null;
  final done = Completer<(String, MachineInfo)?>();
  var pending = hosts.length;
  for (final host in hosts) {
    () async {
      try {
        final info = await fetchInfo(host, port, timeout: timeout, pin: pin);
        if (!done.isCompleted && (expectId == null || info.id == expectId)) done.complete((host, info));
      } catch (_) {
        // unreachable address; others may still answer
      } finally {
        if (--pending == 0 && !done.isCompleted) done.complete(null);
      }
    }();
  }
  return done.future;
}

class PairException implements Exception {
  final String code;
  final String message;
  const PairException(this.code, this.message);
  @override
  String toString() => message;
}

class PairResult {
  final String token;
  final MachineInfo machine;
  const PairResult(this.token, this.machine);
}

/// Exchanges the one-time code for a device token. Call it on exactly one host:
/// every request counts as an attempt and too many wrong ones burn the code.
Future<PairResult> pair(
  String host,
  int port, {
  required String code,
  required String deviceId,
  required String deviceName,
  Pin? pin,
}) async {
  const timeout = Duration(seconds: 5);
  final client = _httpClient(timeout, pin);
  try {
    final req = await client.postUrl(agentUri(host, port, '/api/v1/pair', secure: pin != null)).timeout(timeout);
    req.headers.contentType = ContentType.json;
    req.write(jsonEncode({'code': code, 'deviceId': deviceId, 'deviceName': deviceName}));
    final res = await req.close().timeout(timeout);
    final body = await res.transform(utf8.decoder).join().timeout(timeout);
    switch (res.statusCode) {
      case 200:
        final j = (jsonDecode(body) as Map).cast<String, dynamic>();
        return PairResult(j['token'] as String, MachineInfo.fromJson((j['machine'] as Map).cast<String, dynamic>()));
      case 403:
        throw const PairException('bad_code', 'Неверный код');
      case 410:
        throw const PairException('no_session', 'Код истёк. Нажмите «Подключить телефон» на компьютере ещё раз');
      default:
        throw PairException('http_${res.statusCode}', 'Компьютер ответил ошибкой ${res.statusCode}');
    }
  } on PairException {
    rethrow;
  } on TimeoutException {
    throw const PairException('network', 'Компьютер не отвечает');
  } on SocketException {
    throw const PairException('network', 'Не удалось подключиться к компьютеру');
  } on HandshakeException {
    throw pin?.mismatch == true
        ? const PairException('untrusted', 'Компьютер не совпал с QR-кодом (другой сертификат). Сопряжение отменено')
        : const PairException('network', 'Не удалось установить защищённое соединение');
  } finally {
    client.close(force: true);
  }
}

/// True when the agent rejects [token] (unpaired on the PC), false for any other outcome.
Future<bool> tokenRejected(String host, int port, String token, {Pin? pin}) async {
  const timeout = Duration(seconds: 3);
  final client = _httpClient(timeout, pin);
  try {
    final req = await client.getUrl(agentUri(host, port, '/api/v1/ws', secure: pin != null)).timeout(timeout);
    req.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
    final res = await req.close().timeout(timeout);
    await res.drain<void>();
    return res.statusCode == 401;
  } catch (_) {
    return false;
  } finally {
    client.close(force: true);
  }
}
