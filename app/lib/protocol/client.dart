// HTTP part of the protocol: machine info probing and pairing. Pure Dart.
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'models.dart';

const _probeTimeout = Duration(milliseconds: 1500);

HttpClient _httpClient(Duration timeout) => HttpClient()
  ..connectionTimeout = timeout
  ..userAgent = 'barphone-android/1';

Uri agentUri(String host, int port, String path) => Uri(scheme: 'http', host: host, port: port, path: path);

Future<MachineInfo> fetchInfo(String host, int port, {Duration timeout = _probeTimeout}) async {
  final client = _httpClient(timeout);
  try {
    final req = await client.getUrl(agentUri(host, port, '/api/v1/info')).timeout(timeout);
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
Future<(String, MachineInfo)?> probeHosts(List<String> hosts, int port, {String? expectId, Duration timeout = _probeTimeout}) async {
  if (hosts.isEmpty) return null;
  final done = Completer<(String, MachineInfo)?>();
  var pending = hosts.length;
  for (final host in hosts) {
    () async {
      try {
        final info = await fetchInfo(host, port, timeout: timeout);
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
Future<PairResult> pair(String host, int port, {required String code, required String deviceId, required String deviceName}) async {
  const timeout = Duration(seconds: 5);
  final client = _httpClient(timeout);
  try {
    final req = await client.postUrl(agentUri(host, port, '/api/v1/pair')).timeout(timeout);
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
  } finally {
    client.close(force: true);
  }
}

/// True when the agent rejects [token] (unpaired on the PC), false for any other outcome.
Future<bool> tokenRejected(String host, int port, String token) async {
  const timeout = Duration(seconds: 3);
  final client = _httpClient(timeout);
  try {
    final req = await client.getUrl(agentUri(host, port, '/api/v1/ws')).timeout(timeout);
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
