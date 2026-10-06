// Live WebSocket connection to one paired computer, with reconnection (no widgets).
import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter/foundation.dart' show ValueNotifier;

import 'client.dart';
import 'models.dart';

enum LinkStatus { connecting, online, offline, unauthorized }

/// An open window of a button's app on the PC.
class AppWindow {
  final String id;
  final String title;
  final bool active; // in front on the PC
  const AppWindow(this.id, this.title, {this.active = false});
}

class LaunchResult {
  final bool ok;
  // not_found | launch_failed | window_gone | confirm_required | unsupported | busy | offline | timeout
  final String? error;
  final String? action; // launched | focused | choose | windows | done | volume | minimized
  final List<AppWindow> windows;
  final double? value; // volume level 0..1
  final bool? muted;
  const LaunchResult.ok({this.action, this.windows = const [], this.value, this.muted}) : ok = true, error = null;
  const LaunchResult.fail(this.error) : ok = false, action = null, windows = const [], value = null, muted = null;

  /// Several windows are open: the user has to pick one (or start a new instance).
  bool get needsChoice => ok && (action == 'choose' || action == 'windows');

  factory LaunchResult.fromJson(Map<String, dynamic> m) {
    if (m['ok'] != true) return LaunchResult.fail(m['error'] as String? ?? 'error');
    return LaunchResult.ok(
      action: m['action'] as String?,
      windows: [
        for (final w in (m['windows'] as List?) ?? const [])
          AppWindow((w as Map)['id'] as String, (w['title'] as String?) ?? '', active: w['active'] == true),
      ],
      value: (m['value'] as num?)?.toDouble(),
      muted: m['muted'] as bool?,
    );
  }
}

class MachineLink {
  MachineLink(this._machine, {required this.onChanged, required this.onMachineUpdated}) {
    final cached = _machine.lastState;
    if (cached != null) {
      try {
        state = DeckState.fromJson(cached);
      } catch (_) {}
    }
  }

  /// Called whenever status or state changes.
  final void Function() onChanged;

  /// Called when something worth persisting changed (address that worked, name, MACs, last state).
  final void Function(SavedMachine) onMachineUpdated;

  SavedMachine _machine;
  SavedMachine get machine => _machine;

  LinkStatus status = LinkStatus.offline;
  DeckState? state;

  /// Latest numbers for live tiles; only those tiles listen (it changes every ~2 s).
  final stats = ValueNotifier<SysStats?>(null);
  String? host;

  WebSocket? _ws;
  bool _running = false;
  int _failures = 0;
  Timer? _retry;
  int _seq = 0;
  final _pending = <String, Completer<LaunchResult>>{};

  bool get running => _running;

  /// How long a lost connection is hidden from the user while it comes back on its own.
  static const graceTime = Duration(seconds: 45);

  DateTime? _downSince; // lost the connection at; null while online or if never connected
  Timer? _graceEnd;
  final _onlineWaiters = <Completer<void>>[];

  /// Online, or dropped less than [graceTime] ago and reconnecting in the background: the
  /// screen keeps showing this computer as if nothing happened.
  bool get present {
    if (status == LinkStatus.online) return true;
    final down = _downSince;
    return status != LinkStatus.unauthorized && down != null && DateTime.now().difference(down) < graceTime;
  }

  /// Nothing to tell the user about the connection: [present], or the first attempt is
  /// still under way.
  bool get quiet => present || (status == LinkStatus.connecting && _downSince == null && _failures == 0);

  /// Status for the screen, steady between attempts: a background reconnect reads as
  /// "connecting", a computer that stays away as "offline".
  LinkStatus get shownStatus => switch (status) {
    LinkStatus.online || LinkStatus.unauthorized => status,
    _ => quiet ? LinkStatus.connecting : LinkStatus.offline,
  };

  void _markDown() {
    _downSince = DateTime.now();
    _graceEnd?.cancel();
    _graceEnd = Timer(graceTime, onChanged); // the screen may need to show "offline" now
  }

  /// Replaces stored data (e.g. after re-pairing or a new address from mDNS).
  void update(SavedMachine m, {bool reconnect = false}) {
    final tokenChanged = m.token != _machine.token;
    _machine = m;
    if (tokenChanged || reconnect) {
      _closeSocket();
      if (status == LinkStatus.unauthorized) status = LinkStatus.offline;
      start();
      reconnectNow();
    }
  }

  void start() {
    if (_running) return;
    _running = true;
    if (_downSince != null) _markDown(); // back from the background: a fresh grace period
    _connect();
  }

  void stop() {
    _running = false;
    _retry?.cancel();
    _closeSocket();
    if (status != LinkStatus.unauthorized) _set(LinkStatus.offline);
  }

  /// Skips the backoff wait (app resumed, user tapped the machine, network changed).
  void reconnectNow() {
    if (!_running || _ws != null) return;
    _retry?.cancel();
    _failures = 0;
    _connect();
  }

  bool _connecting = false;

  Future<void> _connect() async {
    if (!_running || _connecting || _ws != null) return;
    _connecting = true;
    if (status != LinkStatus.online) _set(LinkStatus.connecting);
    String? target;
    try {
      final found = await probeHosts(_machine.candidates, _machine.port, expectId: _machine.id);
      if (found == null) throw const SocketException('unreachable');
      target = found.$1;
      final ws = await WebSocket.connect(
        // features=windows: this app can show the window chooser (see docs/protocol.md).
        'ws://$target:${_machine.port}/api/v1/ws?features=windows',
        headers: {HttpHeaders.authorizationHeader: 'Bearer ${_machine.token}'},
      ).timeout(const Duration(seconds: 5));
      if (!_running) {
        ws.close();
        return;
      }
      ws.pingInterval = const Duration(seconds: 10);
      _ws = ws;
      host = target;
      _failures = 0;
      if (_machine.lastHost != target) {
        _machine = _machine.copyWith(lastHost: target);
        onMachineUpdated(_machine);
      }
      _set(LinkStatus.online);
      ws.listen(_onMessage, onDone: () => _onClosed(ws), onError: (_) => _onClosed(ws), cancelOnError: true);
    } on WebSocketException catch (e) {
      if (await _unauthorized(e, target)) {
        _running = false;
        _set(LinkStatus.unauthorized);
      } else {
        _scheduleRetry();
      }
    } catch (_) {
      _scheduleRetry();
    } finally {
      _connecting = false;
    }
  }

  Future<bool> _unauthorized(WebSocketException e, String? target) async {
    final m = RegExp(r'status code: (\d+)').firstMatch(e.message);
    if (m != null) return m.group(1) == '401';
    // Older dart:io does not report the status: ask with a plain request.
    return target != null && await tokenRejected(target, _machine.port, _machine.token);
  }

  void _scheduleRetry() {
    if (!_running) return;
    _failures++;
    _set(LinkStatus.offline);
    final delay = Duration(milliseconds: min(10000, 1000 * pow(2, _failures - 1).toInt()));
    _retry?.cancel();
    _retry = Timer(delay, _connect);
  }

  void _onClosed(WebSocket ws) {
    if (_ws != ws) return;
    _ws = null;
    _failPending('offline');
    if (!_running) return;
    // A server close (e.g. the phone was unpaired) is not final: reconnecting tells us why.
    _set(LinkStatus.connecting);
    _retry?.cancel();
    _retry = Timer(const Duration(milliseconds: 400), _connect);
  }

  void _onMessage(dynamic data) {
    if (data is! String) return;
    final Map<String, dynamic> msg;
    try {
      msg = (jsonDecode(data) as Map).cast<String, dynamic>();
    } catch (_) {
      return;
    }
    switch (msg['type']) {
      case 'state':
        try {
          final st = DeckState.fromJson(msg);
          state = st;
          _machine = _machine.copyWith(
            name: st.machine.name.isEmpty ? null : st.machine.name,
            os: st.machine.os.isEmpty ? null : st.machine.os,
            macs: st.machine.macs.isEmpty ? null : st.machine.macs,
            // New addresses (another network, Tailscale) without pairing again.
            hosts: st.machine.addrs.isEmpty ? null : st.machine.addrs,
            lastState: msg,
          );
          onMachineUpdated(_machine);
          onChanged();
        } catch (_) {}
      case 'stats':
        try {
          stats.value = SysStats.fromJson(msg);
        } catch (_) {}
      case 'result':
        final c = _pending.remove(msg['req']);
        if (c != null && !c.isCompleted) {
          c.complete(LaunchResult.fromJson(msg));
        }
    }
  }

  /// Presses a button. With [newInstance] the PC starts another copy even if the app's
  /// windows are open; otherwise it may switch to the window or ask to choose one.
  /// [confirmed]: the user agreed to a button marked `confirm` (shutdown, restart).
  Future<LaunchResult> launch(String buttonId, {bool newInstance = false, bool confirmed = false}) =>
      _request({'type': 'launch', 'id': buttonId, if (newInstance) 'new': true, if (confirmed) 'confirmed': true});

  /// Reads the PC volume through a slider button, or sets it when [level] is given.
  Future<LaunchResult> volume(String buttonId, [double? level]) =>
      _request({'type': 'volume', 'id': buttonId, if (level != null) 'value': level.clamp(0.0, 1.0)});

  /// Brings up a window picked from [LaunchResult.windows].
  Future<LaunchResult> focus(String buttonId, String windowId) => _request({'type': 'focus', 'id': buttonId, 'window': windowId});

  /// Lists the open windows of the button's app without doing anything (long press).
  Future<LaunchResult> windows(String buttonId) => _request({'type': 'windows', 'id': buttonId});

  /// Minimizes one window of the button's app, or all of them without [windowId].
  Future<LaunchResult> minimize(String buttonId, [String? windowId]) => _request({'type': 'minimize', 'id': buttonId, 'window': ?windowId});

  Future<LaunchResult> _request(Map<String, Object> msg) async {
    if (status != LinkStatus.online && quiet) {
      // A press during a background reconnect waits for it instead of failing.
      reconnectNow();
      await _waitOnline(const Duration(seconds: 4));
    }
    final ws = _ws;
    if (ws == null || status != LinkStatus.online) return const LaunchResult.fail('offline');
    final req = 'r${++_seq}';
    final c = Completer<LaunchResult>();
    _pending[req] = c;
    ws.add(jsonEncode({...msg, 'req': req}));
    return c.future.timeout(
      const Duration(seconds: 5),
      onTimeout: () {
        _pending.remove(req);
        return const LaunchResult.fail('timeout');
      },
    );
  }

  /// Where to fetch icons from, plus the headers the request needs.
  String? iconUrl(String hash) {
    final h = host ?? _machine.lastHost ?? (_machine.hosts.isEmpty ? null : _machine.hosts.first);
    return h == null ? null : 'http://$h:${_machine.port}/api/v1/icon/$hash.png';
  }

  Map<String, String> get authHeaders => {HttpHeaders.authorizationHeader: 'Bearer ${_machine.token}'};

  void _failPending(String error) {
    for (final c in _pending.values) {
      if (!c.isCompleted) c.complete(LaunchResult.fail(error));
    }
    _pending.clear();
  }

  void _closeSocket() {
    final ws = _ws;
    _ws = null;
    _failPending('offline');
    ws?.close();
  }

  Future<void> _waitOnline(Duration limit) {
    final c = Completer<void>();
    _onlineWaiters.add(c);
    return c.future.timeout(limit, onTimeout: () => _onlineWaiters.remove(c));
  }

  void _set(LinkStatus s) {
    if (status == s) return;
    final was = status;
    status = s;
    if (s == LinkStatus.online) {
      _downSince = null;
      _graceEnd?.cancel();
      for (final c in _onlineWaiters) {
        c.complete();
      }
      _onlineWaiters.clear();
    } else if (was == LinkStatus.online) {
      _markDown();
    }
    onChanged();
  }

  void dispose() {
    stop();
    _graceEnd?.cancel();
  }
}
