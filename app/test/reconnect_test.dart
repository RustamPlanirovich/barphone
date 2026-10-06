import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:flutter_test/flutter_test.dart';

/// A tiny stand-in for the agent: /api/v1/info plus a WebSocket that answers presses.
class _Agent {
  late HttpServer _http;
  final sockets = <WebSocket>[];
  bool accepting = true;
  List<String> addrs = const [];
  Map<String, String>? notice; // sent right after the state

  int get port => _http.port;

  Future<void> start([int port = 0]) async {
    _http = await HttpServer.bind(InternetAddress.loopbackIPv4, port);
    _http.listen((req) async {
      if (!accepting) {
        req.response.statusCode = HttpStatus.serviceUnavailable;
        await req.response.close();
        return;
      }
      if (req.uri.path == '/api/v1/info') {
        req.response.write(jsonEncode({'v': 1, 'id': 'm', 'name': 'ПК', 'os': 'windows'}));
        await req.response.close();
        return;
      }
      final ws = await WebSocketTransformer.upgrade(req);
      sockets.add(ws);
      ws.add(
        jsonEncode({
          'type': 'state',
          'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows', 'addrs': addrs},
          'deck': {'columns': 3, 'buttons': <dynamic>[]},
          'recent': <dynamic>[],
        }),
      );
      if (notice != null) ws.add(jsonEncode({'type': 'notify', ...notice!}));
      ws.listen((data) {
        final msg = jsonDecode(data as String) as Map;
        ws.add(jsonEncode({'type': 'result', 'req': msg['req'], 'ok': true, 'action': 'launched'}));
      });
    });
  }

  /// Drops every connection, like a computer whose Wi-Fi hiccups.
  Future<void> drop() async {
    for (final ws in sockets) {
      await ws.close();
    }
    sockets.clear();
  }

  Future<void> stop() => _http.close(force: true);
}

Future<void> until(bool Function() ok, {Duration limit = const Duration(seconds: 5)}) async {
  final end = DateTime.now().add(limit);
  while (!ok()) {
    if (DateTime.now().isAfter(end)) throw TimeoutException('condition not met');
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }
}

void main() {
  test('a dropped connection stays quiet and a press waits for the reconnect', () async {
    final agent = _Agent();
    await agent.start();
    final link = MachineLink(
      SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: agent.port, hosts: const ['127.0.0.1'], token: 't'),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    addTearDown(() async {
      link.dispose();
      await agent.stop();
    });

    expect(link.present, isFalse, reason: 'never connected yet');
    link.start();
    expect(link.quiet, isTrue, reason: 'first attempt: no "offline" banner');
    await until(() => link.status == LinkStatus.online);
    expect(link.present, isTrue);

    // The agent goes away for a moment: the link reconnects in the background.
    agent.accepting = false;
    await agent.drop();
    await until(() => link.status != LinkStatus.online);
    expect(link.present, isTrue);
    expect(link.quiet, isTrue);
    expect(link.shownStatus, LinkStatus.connecting);
    await Future<void>.delayed(const Duration(milliseconds: 900)); // a failed attempt or two
    expect(link.status, isNot(LinkStatus.online));
    expect(link.shownStatus, LinkStatus.connecting, reason: 'steady between attempts');

    // A press now waits for the link instead of failing...
    final press = link.launch('b1');
    await Future<void>.delayed(const Duration(milliseconds: 300));
    agent.accepting = true;
    final res = await press;
    expect(res.ok, isTrue, reason: 'sent once the link was back: ${res.error}');
    expect(link.status, LinkStatus.online);
  });

  test('a press fails as offline when the computer does not come back in time', () async {
    final agent = _Agent();
    await agent.start();
    final link = MachineLink(
      SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: agent.port, hosts: const ['127.0.0.1'], token: 't'),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    addTearDown(() async {
      link.dispose();
      await agent.stop();
    });
    link.start();
    await until(() => link.status == LinkStatus.online);
    agent.accepting = false;
    await agent.drop();
    await until(() => link.status != LinkStatus.online);
    final started = DateTime.now();
    final res = await link.launch('b1');
    expect(res.error, 'offline');
    expect(DateTime.now().difference(started), greaterThan(const Duration(seconds: 3)));
  });

  test('current agent addresses replace the saved ones (Tailscale, a new network)', () async {
    final agent = _Agent()..addrs = ['127.0.0.1', '100.101.12.7'];
    await agent.start();
    final saved = <SavedMachine>[];
    final link = MachineLink(
      SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: agent.port, hosts: const ['127.0.0.1', '192.168.0.99'], token: 't'),
      onChanged: () {},
      onMachineUpdated: saved.add,
    );
    addTearDown(() async {
      link.dispose();
      await agent.stop();
    });
    link.start();
    await until(() => link.state != null);
    expect(link.machine.hosts, ['127.0.0.1', '100.101.12.7']);
    expect(saved.last.hosts, ['127.0.0.1', '100.101.12.7'], reason: 'persisted');
  });

  test('a notice from the PC reaches the app, named after the computer', () async {
    final agent = _Agent()..notice = {'title': 'Сборка', 'text': 'Упала', 'level': 'error'};
    await agent.start();
    final got = <AgentNotice>[];
    final link = MachineLink(
      SavedMachine(id: 'm', name: 'Рабочий ПК', os: 'windows', port: agent.port, hosts: const ['127.0.0.1'], token: 't'),
      onChanged: () {},
      onMachineUpdated: (_) {},
      onNotice: got.add,
    );
    addTearDown(() async {
      link.dispose();
      await agent.stop();
    });
    link.start();
    await until(() => got.isNotEmpty);
    expect([got.single.machine, got.single.title, got.single.text, got.single.level], ['ПК', 'Сборка', 'Упала', 'error']);
  });
}
