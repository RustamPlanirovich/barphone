import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:barphone/protocol/client.dart';
import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:flutter_test/flutter_test.dart';

String _fixture(String name) => File('test/fixtures/$name').readAsStringSync();

/// Fingerprint of a PEM certificate, the way the agent computes it.
String _fpOf(String pem) {
  final body = pem.split('\n').where((l) => l.isNotEmpty && !l.startsWith('-----')).join();
  return certFingerprint(base64.decode(body));
}

/// A stand-in agent: TLS with the test certificate ([secure]) or plain HTTP.
class _Agent {
  _Agent({required this.secure, this.stateFp});
  final bool secure;
  final String? stateFp; // what "state.tls.fp" says
  late HttpServer _http;
  int wsConnections = 0;

  int get port => _http.port;

  Future<void> start() async {
    _http = secure
        ? await HttpServer.bindSecure(
            InternetAddress.loopbackIPv4,
            0,
            SecurityContext()
              ..useCertificateChainBytes(utf8.encode(_fixture('agent-cert.pem')))
              ..usePrivateKeyBytes(utf8.encode(_fixture('agent-key.pem'))),
          )
        : await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    _http.listen((req) async {
      switch (req.uri.path) {
        case '/api/v1/info':
          req.response.write(jsonEncode({'v': 1, 'id': 'm', 'name': 'ПК', 'os': 'windows'}));
          await req.response.close();
        case '/api/v1/pair':
          final body = jsonDecode(await utf8.decoder.bind(req).join()) as Map;
          req.response.statusCode = body['code'] == '123456' ? 200 : 403;
          req.response.write(
            jsonEncode({
              'token': 'tok',
              'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
            }),
          );
          await req.response.close();
        default:
          final ws = await WebSocketTransformer.upgrade(req);
          wsConnections++;
          ws.add(
            jsonEncode({
              'type': 'state',
              'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
              'deck': {'columns': 3, 'buttons': <dynamic>[]},
              'recent': <dynamic>[],
              if (stateFp != null) 'tls': {'fp': stateFp},
            }),
          );
          ws.listen((_) {});
      }
    });
  }

  Future<void> stop() => _http.close(force: true);
}

Future<void> until(bool Function() ok, {Duration limit = const Duration(seconds: 6)}) async {
  final end = DateTime.now().add(limit);
  while (!ok()) {
    if (DateTime.now().isAfter(end)) throw TimeoutException('condition not met');
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }
}

void main() {
  final agentFp = _fpOf(_fixture('agent-cert.pem'));
  final otherFp = _fpOf(_fixture('other-cert.pem'));

  Future<(_Agent, MachineLink, List<SavedMachine>)> connect({required bool secure, String? fp, bool tlsOk = false, String? stateFp}) async {
    final agent = _Agent(secure: secure, stateFp: stateFp);
    await agent.start();
    final saved = <SavedMachine>[];
    final link = MachineLink(
      SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: agent.port, hosts: const ['127.0.0.1'], token: 't', fp: fp, tlsOk: tlsOk),
      onChanged: () {},
      onMachineUpdated: saved.add,
    );
    addTearDown(() async {
      link.dispose();
      await agent.stop();
    });
    link.start();
    return (agent, link, saved);
  }

  test('the QR fingerprint pins pairing: the right certificate pairs, another does not', () async {
    final agent = _Agent(secure: true);
    await agent.start();
    addTearDown(agent.stop);
    expect(agentFp, hasLength(43));

    final ok = Pin(agentFp);
    expect(await probeHosts(['127.0.0.1'], agent.port, expectId: 'm', pin: ok), isNotNull);
    final r = await pair('127.0.0.1', agent.port, code: '123456', deviceId: 'd', deviceName: 'Pixel', pin: ok);
    expect(r.token, 'tok');

    final wrong = Pin(otherFp);
    expect(await probeHosts(['127.0.0.1'], agent.port, expectId: 'm', pin: wrong), isNull);
    expect(wrong.mismatch, isTrue);
    await expectLater(
      pair('127.0.0.1', agent.port, code: '123456', deviceId: 'd', deviceName: 'Pixel', pin: Pin(otherFp)),
      throwsA(isA<PairException>().having((e) => e.code, 'code', 'untrusted')),
    );
  });

  test('pinned and not yet confirmed: the first TLS connection confirms it for good', () async {
    final (_, link, saved) = await connect(secure: true, fp: agentFp);
    await until(() => link.status == LinkStatus.online);
    expect(link.secure, isTrue);
    expect(link.machine.tlsOk, isTrue);
    expect(saved.last.tlsOk, isTrue, reason: 'persisted');
    expect(link.iconUrl('abc'), startsWith('https://'));
  });

  test('another certificate: "not recognized", no reconnecting, whether confirmed or not', () async {
    for (final confirmed in [true, false]) {
      final (agent, link, _) = await connect(secure: true, fp: otherFp, tlsOk: confirmed);
      await until(() => link.status == LinkStatus.untrusted);
      expect(link.running, isFalse);
      expect(agent.wsConnections, 0);
    }
  });

  test('before the first TLS success a plain agent is still reachable; after it, never in the clear', () async {
    final (_, soft, _) = await connect(secure: false, fp: agentFp);
    await until(() => soft.status == LinkStatus.online);
    expect(soft.secure, isFalse);
    expect(soft.machine.tlsOk, isFalse);

    final (plain, hard, _) = await connect(secure: false, fp: agentFp, tlsOk: true);
    await Future<void>.delayed(const Duration(seconds: 2));
    expect(hard.status, isNot(LinkStatus.online));
    expect(hard.status, isNot(LinkStatus.untrusted), reason: 'no certificate at all is not a mismatch');
    expect(plain.wsConnections, 0, reason: 'no plain connection once TLS has worked');
  });

  test('paired without a fingerprint: the agent tells it in its state (and TLS comes next)', () async {
    final (_, link, saved) = await connect(secure: false, stateFp: agentFp);
    await until(() => link.state != null);
    expect(link.machine.fp, agentFp);
    expect(link.machine.tlsOk, isFalse);
    expect(saved.last.fp, agentFp);
  });

  test('a fingerprint from the QR is not replaced by what a plain connection says', () async {
    final (_, link, _) = await connect(secure: false, fp: agentFp, stateFp: otherFp);
    await until(() => link.state != null);
    expect(link.machine.fp, agentFp);
  });

  test('pictures over HTTPS: any certificate a paired computer is pinned to, nothing else', () async {
    final agent = _Agent(secure: true);
    await agent.start();
    addTearDown(agent.stop);
    Future<int?> get(Set<String> pinned) => HttpOverrides.runWithHttpOverrides(() async {
      final c = HttpClient();
      try {
        final res = await (await c.getUrl(Uri.parse('https://127.0.0.1:${agent.port}/api/v1/info'))).close();
        await res.drain<void>();
        return res.statusCode;
      } on HandshakeException {
        return null;
      } finally {
        c.close(force: true);
      }
    }, PinningOverrides(() => pinned));
    expect(await get({otherFp, agentFp}), 200);
    expect(await get({otherFp}), isNull);
  });
}
