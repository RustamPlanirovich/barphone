import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:device_info_plus/device_info_plus.dart';
import 'package:flutter/widgets.dart';
import 'package:nsd/nsd.dart' as nsd;
import 'package:shared_preferences/shared_preferences.dart';

import 'timers.dart';
import 'protocol/client.dart';
import 'protocol/link.dart';
import 'protocol/models.dart';
import 'protocol/wol.dart' as wol;

/// An agent seen via mDNS.
class DiscoveredAgent {
  final String id;
  final String name;
  final String os;
  final int port;
  final List<String> hosts;
  const DiscoveredAgent(this.id, this.name, this.os, this.port, this.hosts);
}

class AppState extends ChangeNotifier with WidgetsBindingObserver {
  AppState({DeckTimers? timers}) : timers = timers ?? DeckTimers();

  /// Timer buttons count down here, on the phone.
  final DeckTimers timers;

  late SharedPreferences _prefs;
  String deviceId = '';
  String deviceName = 'Android';

  final List<String> _order = [];
  final Map<String, MachineLink> _links = {};
  String? _activeId;

  nsd.Discovery? _discovery;
  List<DiscoveredAgent> discovered = const [];

  bool _resumed = true;

  List<MachineLink> get machines => [for (final id in _order) _links[id]!];
  MachineLink? get active => _links[_activeId] ?? (_order.isEmpty ? null : _links[_order.first]);
  bool isPaired(String machineId) => _links.containsKey(machineId);

  /// The computer shown next to the active one on a landscape screen: the first other one
  /// that is connected while the active one is too.
  MachineLink? get companion {
    final a = active;
    if (a == null || !a.present) return null;
    for (final id in _order) {
      final l = _links[id]!;
      if (l != a && l.present) return l;
    }
    return null;
  }

  Future<void> init() async {
    _prefs = await SharedPreferences.getInstance();
    deviceId = _prefs.getString('device_id') ?? _newId();
    await _prefs.setString('device_id', deviceId);
    deviceName = await _readDeviceName();

    final raw = _prefs.getString('machines');
    if (raw != null) {
      try {
        for (final j in jsonDecode(raw) as List) {
          final m = SavedMachine.fromJson((j as Map).cast<String, dynamic>());
          _order.add(m.id);
          _links[m.id] = _newLink(m);
        }
      } catch (_) {
        // corrupt storage: start clean rather than crash
      }
    }
    _activeId = _prefs.getString('active');
    WidgetsBinding.instance.addObserver(this);
    for (final l in _links.values) {
      l.start();
    }
    _startDiscovery();
  }

  static String _newId() {
    final r = Random.secure();
    return List.generate(16, (_) => r.nextInt(256).toRadixString(16).padLeft(2, '0')).join();
  }

  static Future<String> _readDeviceName() async {
    try {
      final info = await DeviceInfoPlugin().androidInfo;
      final model = info.model.replaceAll('_', ' ').trim();
      final brand = info.brand.trim();
      if (model.isEmpty) return 'Android';
      if (brand.isEmpty || model.toLowerCase().contains(brand.toLowerCase())) return model;
      return '${brand[0].toUpperCase()}${brand.substring(1)} $model';
    } catch (_) {
      return 'Android';
    }
  }

  MachineLink _newLink(SavedMachine m) =>
      MachineLink(m, onChanged: notifyListeners, onMachineUpdated: (_) => _persist(), onNotice: _notices.add);

  final _notices = StreamController<AgentNotice>.broadcast();

  /// Notices from every computer, for the home screen to show.
  Stream<AgentNotice> get notices => _notices.stream;

  /// Shows a notice as if a computer had sent it (tests, and the phone's own messages).
  void notice(AgentNotice n) => _notices.add(n);

  Timer? _persistTimer;
  void _persist() {
    _persistTimer?.cancel();
    _persistTimer = Timer(const Duration(milliseconds: 300), () {
      _prefs.setString('machines', jsonEncode([for (final l in machines) l.machine.toJson()]));
      if (_activeId != null) _prefs.setString('active', _activeId!);
    });
  }

  // ---- lifecycle ----------------------------------------------------------

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final resumed = state == AppLifecycleState.resumed;
    if (resumed == _resumed) return;
    _resumed = resumed;
    if (resumed) {
      timers.check(); // a timer may have run out while the app was away
      for (final l in _links.values) {
        if (l.status != LinkStatus.unauthorized && l.status != LinkStatus.untrusted) l.start();
        l.reconnectNow();
      }
      _startDiscovery();
    } else if (state == AppLifecycleState.paused || state == AppLifecycleState.hidden) {
      for (final l in _links.values) {
        l.stop();
      }
      _stopDiscovery();
    } else {
      _resumed = true; // inactive (e.g. notification shade): keep connections
    }
  }

  // ---- machines -----------------------------------------------------------

  void setActive(String id) {
    if (!_links.containsKey(id)) return;
    _activeId = id;
    _links[id]!.reconnectNow();
    _persist();
    notifyListeners();
  }

  /// Pins a profile of machine [id] on this phone; null goes back to following the PC.
  void setPinnedProfile(String id, String? profileId) {
    final l = _links[id];
    if (l == null || l.machine.pinnedProfile == profileId) return;
    l.update(l.machine.copyWith(pinnedProfile: (profileId,)));
    _persist();
    notifyListeners();
  }

  void removeMachine(String id) {
    final l = _links.remove(id);
    l?.dispose();
    _order.remove(id);
    if (_activeId == id) _activeId = _order.isEmpty ? null : _order.first;
    _persist();
    notifyListeners();
  }

  Future<bool> wake(String id) async {
    final l = _links[id];
    if (l == null || l.machine.macs.isEmpty) return false;
    final sent = await wol.wake(l.machine.macs, lastHost: l.machine.lastHost);
    l.reconnectNow();
    return sent > 0;
  }

  void _saveMachine(SavedMachine m) {
    final existing = _links[m.id];
    if (existing != null) {
      existing.update(m, reconnect: true);
    } else {
      _order.add(m.id);
      final l = _newLink(m);
      _links[m.id] = l;
      l.start();
    }
    _activeId = m.id;
    _persist();
    notifyListeners();
  }

  // ---- pairing ------------------------------------------------------------
  // Each method returns null on success or a message for the user.

  /// With a fingerprint in the QR code, pairing (and everything after it) goes over TLS
  /// pinned to it: the code and the token never travel in the clear.
  Future<String?> pairWithUri(PairUri p) async {
    final fp = p.fp;
    final pin = fp == null ? null : Pin(fp);
    final found = await probeHosts(p.hosts, p.port, expectId: p.machineId, pin: pin);
    if (pin != null && pin.mismatch) {
      return 'Компьютер по этому адресу не совпал с QR-кодом (другой сертификат). Сопряжение отменено.';
    }
    if (found == null) {
      return 'Компьютер «${p.name}» не отвечает. Телефон и компьютер должны быть в одной сети, '
          'а брандмауэр на компьютере — пропускать barphone.';
    }
    return _pairAt(found.$1, p.port, p.code, [found.$1, ...p.hosts.where((h) => h != found.$1)], pin: pin);
  }

  Future<String?> pairDiscovered(DiscoveredAgent a, String code) async {
    final found = await probeHosts(a.hosts, a.port, expectId: a.id);
    if (found == null) return 'Компьютер «${a.name}» не отвечает.';
    return _pairAt(found.$1, a.port, code, a.hosts);
  }

  Future<String?> pairManual(String address, String code) async {
    final parts = address.trim().split(':');
    final host = parts.first;
    final port = parts.length > 1 ? int.tryParse(parts[1]) ?? defaultPort : defaultPort;
    if (InternetAddress.tryParse(host) == null) return 'Введите IP-адрес, например 192.168.1.20';
    final found = await probeHosts([host], port);
    if (found == null) return 'По адресу $host:$port barphone не отвечает.';
    return _pairAt(host, port, code, [host]);
  }

  Future<String?> _pairAt(String host, int port, String code, List<String> hosts, {Pin? pin}) async {
    try {
      final r = await pair(host, port, code: code.trim(), deviceId: deviceId, deviceName: deviceName, pin: pin);
      final prev = _links[r.machine.id]?.machine;
      _saveMachine(
        SavedMachine(
          id: r.machine.id,
          name: r.machine.name,
          os: r.machine.os,
          port: port,
          hosts: {...hosts, ...?prev?.hosts}.toList(),
          token: r.token,
          macs: prev?.macs ?? const [],
          lastHost: host,
          lastState: prev?.lastState,
          pinnedProfile: prev?.pinnedProfile,
          // Paired over TLS: pinned for good. Without a QR fingerprint the agent tells it
          // in its first state (and TLS is tried from then on).
          fp: pin?.fp,
          tlsOk: pin != null,
        ),
      );
      return null;
    } on PairException catch (e) {
      return e.message;
    }
  }

  // ---- discovery ----------------------------------------------------------

  Future<void> _startDiscovery() async {
    if (_discovery != null) return;
    try {
      final d = await nsd.startDiscovery('_barphone._tcp', ipLookupType: nsd.IpLookupType.v4);
      _discovery = d;
      d.addListener(_onDiscovery);
    } catch (_) {
      // NSD unavailable: QR and manual pairing still work
    }
  }

  Future<void> _stopDiscovery() async {
    final d = _discovery;
    _discovery = null;
    if (d == null) return;
    d.removeListener(_onDiscovery);
    try {
      await nsd.stopDiscovery(d);
    } catch (_) {}
  }

  void _onDiscovery() {
    final d = _discovery;
    if (d == null) return;
    String txt(nsd.Service s, String key) {
      final v = s.txt?[key];
      return v == null ? '' : utf8.decode(v, allowMalformed: true);
    }

    final found = <DiscoveredAgent>[];
    for (final s in d.services) {
      final id = txt(s, 'id');
      final hosts = [
        for (final a in s.addresses ?? const <InternetAddress>[])
          if (a.type == InternetAddressType.IPv4) a.address,
      ];
      if (id.isEmpty || hosts.isEmpty || s.port == null) continue;
      found.add(DiscoveredAgent(id, txt(s, 'name').isEmpty ? (s.name ?? id) : txt(s, 'name'), txt(s, 'os'), s.port!, hosts));

      // A paired machine changed its address (DHCP): learn the new one.
      final l = _links[id];
      if (l != null && hosts.any((h) => !l.machine.hosts.contains(h))) {
        l.update(l.machine.copyWith(hosts: {...hosts, ...l.machine.hosts}.toList()), reconnect: l.status != LinkStatus.online);
        _persist();
      }
    }
    discovered = found;
    notifyListeners();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _stopDiscovery();
    for (final l in _links.values) {
      l.dispose();
    }
    timers.dispose();
    _notices.close();
    super.dispose();
  }
}
