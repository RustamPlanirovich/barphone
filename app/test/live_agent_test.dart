// Talks to a running agent on this machine exactly like the phone does.
// Skipped unless BARPHONE_LIVE=1:  BARPHONE_LIVE=1 flutter test test/live_agent_test.dart
// BARPHONE_UI points at a non-default agent (e.g. http://127.0.0.1:47901); BARPHONE_PROBE
// at a built agent/tools/probewin.exe enables the window-chooser test.
import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:barphone/protocol/client.dart';
import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:flutter_test/flutter_test.dart';

final ui = Platform.environment['BARPHONE_UI'] ?? 'http://127.0.0.1:47801';

Future<dynamic> uiCall(String method, String path, [Object? body]) async {
  final client = HttpClient();
  try {
    final req = await client.openUrl(method, Uri.parse('$ui$path'));
    req.headers.set('X-Barphone-UI', '1');
    if (body != null) {
      req.headers.contentType = ContentType.json;
      req.write(jsonEncode(body));
    }
    final res = await req.close();
    final text = await res.transform(utf8.decoder).join();
    if (res.statusCode >= 300) throw HttpException('$method $path: ${res.statusCode} $text');
    return text.isEmpty ? null : jsonDecode(text);
  } finally {
    client.close();
  }
}

Future<void> waitFor(bool Function() cond, {Duration timeout = const Duration(seconds: 8)}) async {
  final end = DateTime.now().add(timeout);
  while (!cond()) {
    if (DateTime.now().isAfter(end)) throw TimeoutException('condition not met');
    await Future<void>.delayed(const Duration(milliseconds: 50));
  }
}

void main() {
  final live = Platform.environment['BARPHONE_LIVE'] == '1';

  setUpAll(() => HttpOverrides.global = null);

  test('pair, receive deck, launch, get unpaired', () async {
    final st = (await uiCall('GET', '/api/state')) as Map<String, dynamic>;
    final originalDeck = st['deck'];
    final machineId = st['machine']['id'] as String;
    final hosts = [for (final a in st['lan']['addrs'] as List) a['ip'] as String];
    final port = st['lan']['port'] as int;

    // A probe button that launches nothing visible.
    final buttons = [
      ...(originalDeck['buttons'] as List),
      {'title': 'probe', 'kind': 'path', 'target': 'rundll32.exe'},
    ];
    await uiCall('PUT', '/api/deck', {'columns': originalDeck['columns'], 'buttons': buttons});

    try {
      final pairing = (await uiCall('POST', '/api/pairing')) as Map<String, dynamic>;
      final uri = PairUri.parse(pairing['uri'] as String)!;
      expect(uri.machineId, machineId);

      final found = await probeHosts(uri.hosts, uri.port, expectId: uri.machineId);
      expect(found, isNotNull, reason: 'no LAN address of the agent answered');
      final res = await pair(found!.$1, port, code: uri.code, deviceId: 'flutter-live-test', deviceName: 'Flutter test');
      expect(res.machine.id, machineId);

      final changes = StreamController<void>.broadcast();
      final link = MachineLink(
        SavedMachine(id: machineId, name: res.machine.name, os: res.machine.os, port: port, hosts: hosts, token: res.token),
        onChanged: () => changes.add(null),
        onMachineUpdated: (_) {},
      );
      link.start();
      await waitFor(() => link.status == LinkStatus.online && link.state != null);
      final probe = link.state!.buttons.firstWhere((b) => b.title == 'probe');

      final ok = await link.launch(probe.id);
      expect(ok.ok, isTrue, reason: ok.error);
      await waitFor(() => link.state!.recent.isNotEmpty && link.state!.recent.first.id == probe.id);
      expect((await link.launch('nope')).error, 'not_found');

      // Icon endpoint works with the link's headers.
      await waitFor(() => link.state!.buttons.firstWhere((b) => b.id == probe.id).icon != null);
      final hash = link.state!.buttons.firstWhere((b) => b.id == probe.id).icon!;
      final client = HttpClient();
      final req = await client.getUrl(Uri.parse(link.iconUrl(hash)!));
      link.authHeaders.forEach(req.headers.set);
      final iconRes = await req.close();
      await iconRes.drain<void>();
      client.close();
      expect(iconRes.statusCode, 200);

      // Unpairing on the PC flips the link to "unauthorized" instead of retrying forever.
      await uiCall('DELETE', '/api/devices/flutter-live-test');
      await waitFor(() => link.status == LinkStatus.unauthorized);
      link.dispose();
      await changes.close();
    } finally {
      await uiCall('PUT', '/api/deck', originalDeck);
      try {
        await uiCall('DELETE', '/api/devices/flutter-live-test');
      } catch (_) {}
    }
  }, skip: live ? false : 'set BARPHONE_LIVE=1 with a running agent');

  final probe = Platform.environment['BARPHONE_PROBE'];

  test('several open windows: choose one, switch, or start a new instance', () async {
    Future<String> foreground() async => ((await Process.run(probe!, ['-fg'])).stdout as String).trim();
    final windowsProc = await Process.start(probe!, ['Probe A - barphone probe', 'Probe B - barphone probe']);
    final st = (await uiCall('GET', '/api/state')) as Map<String, dynamic>;
    final originalDeck = st['deck'];
    final machineId = st['machine']['id'] as String;
    final hosts = [for (final a in st['lan']['addrs'] as List) a['ip'] as String];
    final port = st['lan']['port'] as int;
    await uiCall('PUT', '/api/deck', {
      'columns': 3,
      'buttons': [
        {'title': 'probe', 'kind': 'path', 'target': probe},
      ],
    });
    try {
      final pairing = (await uiCall('POST', '/api/pairing')) as Map<String, dynamic>;
      final uri = PairUri.parse(pairing['uri'] as String)!;
      final found = await probeHosts(uri.hosts, uri.port, expectId: uri.machineId);
      final res = await pair(found!.$1, port, code: uri.code, deviceId: 'flutter-live-windows', deviceName: 'Flutter windows test');
      final link = MachineLink(
        SavedMachine(id: machineId, name: res.machine.name, os: res.machine.os, port: port, hosts: hosts, token: res.token),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
      link.start();
      await waitFor(() => link.status == LinkStatus.online && link.state != null && link.state!.buttons.isNotEmpty);
      final id = link.state!.buttons.single.id;

      // Both probe windows are found; the press asks instead of launching.
      for (var i = 0; i < 40 && (await link.windows(id)).windows.length < 2; i++) {
        await Future<void>.delayed(const Duration(milliseconds: 150));
      }
      final r = await link.launch(id);
      expect(r.needsChoice, isTrue, reason: 'action=${r.action} windows=${r.windows.length}');
      expect(r.windows.map((w) => w.title).toSet(), {'Probe A', 'Probe B'}, reason: 'shared suffix is trimmed');

      for (final w in r.windows.reversed) {
        final f = await link.focus(id, w.id);
        expect(f.ok, isTrue, reason: f.error);
        expect(f.action, 'focused');
        await Future<void>.delayed(const Duration(milliseconds: 400));
        expect(await foreground(), '${w.title} - barphone probe');
      }
      expect((await link.focus(id, '1')).error, 'window_gone');

      // Long press lists the windows without doing anything; "new" starts another instance.
      expect((await link.windows(id)).windows.length, 2);
      final n = await link.launch(id, newInstance: true);
      expect(n.action, 'launched');
      await waitFor(() => link.state!.recent.isNotEmpty);
      link.dispose();
    } finally {
      windowsProc.kill();
      await Process.run('taskkill', ['/F', '/IM', 'probewin.exe']);
      await uiCall('PUT', '/api/deck', originalDeck);
      try {
        await uiCall('DELETE', '/api/devices/flutter-live-windows');
      } catch (_) {}
    }
  }, skip: live && probe != null ? false : 'set BARPHONE_LIVE=1 and BARPHONE_PROBE');

  // Safe on a real PC: reads the volume and writes the same level back, and checks that
  // shutdown is refused without confirmation. It never sends a confirmed shutdown.
  test('volume slider round trip; shutdown needs confirmation', () async {
    final st = (await uiCall('GET', '/api/state')) as Map<String, dynamic>;
    final originalDeck = st['deck'];
    final machineId = st['machine']['id'] as String;
    final hosts = [for (final a in st['lan']['addrs'] as List) a['ip'] as String];
    final port = st['lan']['port'] as int;
    await uiCall('PUT', '/api/deck', {
      'columns': 3,
      'buttons': [
        {'kind': 'system', 'target': 'volume'},
        {'kind': 'system', 'target': 'shutdown'},
      ],
    });
    try {
      final pairing = (await uiCall('POST', '/api/pairing')) as Map<String, dynamic>;
      final uri = PairUri.parse(pairing['uri'] as String)!;
      final found = await probeHosts(uri.hosts, uri.port, expectId: uri.machineId);
      final res = await pair(found!.$1, port, code: uri.code, deviceId: 'flutter-live-actions', deviceName: 'Flutter actions test');
      final link = MachineLink(
        SavedMachine(id: machineId, name: res.machine.name, os: res.machine.os, port: port, hosts: hosts, token: res.token),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
      link.start();
      await waitFor(() => link.status == LinkStatus.online && link.state != null && link.state!.buttons.length == 2);
      final volume = link.state!.buttons.firstWhere((b) => b.glyph == 'volume');
      final shutdown = link.state!.buttons.firstWhere((b) => b.glyph == 'shutdown');
      expect(volume.isSlider, isTrue);
      expect(shutdown.confirm, isTrue);

      final read = await link.volume(volume.id);
      expect(read.ok, isTrue, reason: read.error);
      if (read.muted != true) {
        final write = await link.volume(volume.id, read.value);
        expect(write.value, closeTo(read.value!, .01));
      }
      expect((await link.launch(shutdown.id)).error, 'confirm_required');
      link.dispose();
    } finally {
      await uiCall('PUT', '/api/deck', originalDeck);
      try {
        await uiCall('DELETE', '/api/devices/flutter-live-actions');
      } catch (_) {}
    }
  }, skip: live ? false : 'set BARPHONE_LIVE=1 with a running agent');

  // Read-only on the desktop: binds a profile to whatever app is in front right now and
  // expects the phone side to receive that profile as active. Nothing is focused.
  test('profile bound to the app in front becomes active on the phone', () async {
    final st = (await uiCall('GET', '/api/state')) as Map<String, dynamic>;
    final fg = st['foreground'] as Map<String, dynamic>?;
    if (fg == null) {
      markTestSkipped('nothing identifiable in front on the PC right now');
      return;
    }
    final originalProfiles = st['profiles'] as List;
    final machineId = st['machine']['id'] as String;
    final hosts = [for (final a in st['lan']['addrs'] as List) a['ip'] as String];
    final port = st['lan']['port'] as int;
    final saved = (await uiCall('PUT', '/api/profiles', [
      ...originalProfiles,
      {
        'name': 'Live test',
        'apps': [
          {'name': fg['name'], 'keys': fg['keys']},
        ],
        'deck': {
          'columns': 3,
          'buttons': [
            {'kind': 'system', 'target': 'mute'},
          ],
        },
      },
    ])) as List;
    final liveId = (saved.last as Map)['id'] as String;
    try {
      final pairing = (await uiCall('POST', '/api/pairing')) as Map<String, dynamic>;
      final uri = PairUri.parse(pairing['uri'] as String)!;
      final found = await probeHosts(uri.hosts, uri.port, expectId: uri.machineId);
      final res = await pair(found!.$1, port, code: uri.code, deviceId: 'flutter-live-profiles', deviceName: 'Flutter profiles test');
      final link = MachineLink(
        SavedMachine(id: machineId, name: res.machine.name, os: res.machine.os, port: port, hosts: hosts, token: res.token),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
      link.start();
      await waitFor(() => link.state?.profile(liveId) != null);
      // The user may switch apps meanwhile; only assert when the same app is still in front.
      final now = (await uiCall('GET', '/api/state')) as Map<String, dynamic>;
      if ((now['foreground'] as Map?)?['name'] == fg['name']) {
        await waitFor(() => link.state!.activeProfile == liveId);
        expect(link.state!.shown(null).name, 'Live test');
        expect(link.state!.buttons.single.glyph, 'mute');
      }
      expect(link.state!.shown(defaultProfileId).id, defaultProfileId, reason: 'pinning works locally');
      link.dispose();
    } finally {
      await uiCall('PUT', '/api/profiles', originalProfiles);
      try {
        await uiCall('DELETE', '/api/devices/flutter-live-profiles');
      } catch (_) {}
    }
  }, skip: live ? false : 'set BARPHONE_LIVE=1 with a running agent');
}
