// Wire and storage models for the barphone protocol (see docs/protocol.md).
// Pure Dart: no Flutter imports, so it can be tested on the host.

const int defaultPort = 47800;
const int protocolVersion = 1;

class MachineInfo {
  final String id;
  final String name;
  final String os;
  final List<String> macs;

  const MachineInfo({required this.id, required this.name, required this.os, this.macs = const []});

  factory MachineInfo.fromJson(Map<String, dynamic> j) => MachineInfo(
    id: j['id'] as String,
    name: (j['name'] as String?) ?? '',
    os: (j['os'] as String?) ?? '',
    macs: ((j['macs'] as List?) ?? const []).cast<String>(),
  );
}

class DeckButton {
  final String id;
  final String title;
  final String kind; // app | path | url | keys | text | system | folder | macro
  final String? icon;
  final String? glyph; // built-in picture: "keys", "text" or a system action id
  final String? control; // "slider": hold and drag (volume)
  final bool confirm; // ask before pressing (shutdown, restart)
  final List<DeckButton> buttons; // a folder's buttons

  const DeckButton({
    required this.id,
    required this.title,
    required this.kind,
    this.icon,
    this.glyph,
    this.control,
    this.confirm = false,
    this.buttons = const [],
  });

  /// Starts a program on the PC, so it can have open windows to choose from.
  bool get launchesApp => kind == 'app' || kind == 'path';
  bool get isSlider => control == 'slider';

  /// Opens on the phone, showing its own buttons; the agent is not asked.
  bool get isFolder => kind == 'folder';

  factory DeckButton.fromJson(Map<String, dynamic> j) => DeckButton(
    id: j['id'] as String,
    title: (j['title'] as String?) ?? '',
    kind: (j['kind'] as String?) ?? 'app',
    icon: j['icon'] as String?,
    glyph: j['glyph'] as String?,
    control: j['control'] as String?,
    confirm: j['confirm'] == true,
    buttons: ((j['buttons'] as List?) ?? const []).map((b) => DeckButton.fromJson((b as Map).cast<String, dynamic>())).toList(),
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'title': title,
    'kind': kind,
    'icon': icon,
    if (glyph != null) 'glyph': glyph,
    if (control != null) 'control': control,
    if (confirm) 'confirm': true,
    if (buttons.isNotEmpty) 'buttons': [for (final b in buttons) b.toJson()],
  };
}

class RecentEntry {
  final String id;
  final DateTime at;

  const RecentEntry(this.id, this.at);

  factory RecentEntry.fromJson(Map<String, dynamic> j) =>
      RecentEntry(j['id'] as String, DateTime.tryParse(j['at'] as String? ?? '')?.toLocal() ?? DateTime.now());

  Map<String, dynamic> toJson() => {'id': id, 'at': at.toUtc().toIso8601String()};
}

/// One deck of buttons on the PC. Which one the phone shows depends on the app in front
/// on the PC (or on the profile pinned on the phone).
class DeckProfile {
  final String id;
  final String name;
  final int columns;
  final List<DeckButton> buttons;

  const DeckProfile({required this.id, required this.name, required this.columns, required this.buttons});

  factory DeckProfile.fromJson(Map<String, dynamic> j) => DeckProfile(
    id: j['id'] as String,
    name: (j['name'] as String?) ?? '',
    columns: ((j['columns'] as num?) ?? 3).toInt().clamp(1, 12),
    buttons: ((j['buttons'] as List?) ?? const []).map((b) => DeckButton.fromJson((b as Map).cast<String, dynamic>())).toList(),
  );
}

const defaultProfileId = 'default';

class DeckState {
  final MachineInfo machine;
  final List<DeckProfile> profiles; // never empty; agents without profiles give one
  final String activeProfile; // chosen by the PC from the app in front
  final List<RecentEntry> recent;

  const DeckState({required this.machine, required this.profiles, required this.activeProfile, required this.recent});

  factory DeckState.fromJson(Map<String, dynamic> j) {
    final deck = (j['deck'] as Map?)?.cast<String, dynamic>() ?? const {};
    var profiles = ((j['profiles'] as List?) ?? const []).map((p) => DeckProfile.fromJson((p as Map).cast<String, dynamic>())).toList();
    if (profiles.isEmpty) {
      // An agent from before profiles: its single deck is the default profile.
      profiles = [
        DeckProfile.fromJson({...deck, 'id': defaultProfileId, 'name': 'Основной'}),
      ];
    }
    return DeckState(
      machine: MachineInfo.fromJson((j['machine'] as Map).cast<String, dynamic>()),
      profiles: profiles,
      activeProfile: (j['activeProfile'] as String?) ?? profiles.first.id,
      recent: ((j['recent'] as List?) ?? const []).map((r) => RecentEntry.fromJson((r as Map).cast<String, dynamic>())).toList(),
    );
  }

  DeckProfile? profile(String? id) {
    for (final p in profiles) {
      if (p.id == id) return p;
    }
    return null;
  }

  /// The profile to show: the pinned one if it still exists, else the PC's choice,
  /// else the default.
  DeckProfile shown(String? pinned) => profile(pinned) ?? profile(activeProfile) ?? profiles.first;

  /// The PC's current choice (used by apps that show a single deck).
  int get columns => shown(null).columns;
  List<DeckButton> get buttons => shown(null).buttons;

  /// Finds a button in any profile, folders included.
  DeckButton? button(String id) {
    DeckButton? find(List<DeckButton> list) {
      for (final b in list) {
        if (b.id == id) return b;
        final inside = find(b.buttons);
        if (inside != null) return inside;
      }
      return null;
    }

    for (final p in profiles) {
      final b = find(p.buttons);
      if (b != null) return b;
    }
    return null;
  }

  /// Recent launches resolved to buttons that still exist, newest first.
  List<(DeckButton, DateTime)> get recentButtons => [
    for (final r in recent)
      if (button(r.id) case final b?) (b, r.at),
  ];
}

/// Contents of the pairing QR code / deep link:
/// barphone://pair?v=1&id=..&name=..&os=..&port=47800&code=123456&ip=a,b
class PairUri {
  final String machineId;
  final String name;
  final String os;
  final int port;
  final String code;
  final List<String> hosts;

  const PairUri({
    required this.machineId,
    required this.name,
    required this.os,
    required this.port,
    required this.code,
    required this.hosts,
  });

  static PairUri? parse(String raw) {
    final uri = Uri.tryParse(raw.trim());
    if (uri == null || uri.scheme != 'barphone' || uri.host != 'pair') return null;
    final q = uri.queryParameters;
    final id = q['id'] ?? '';
    final code = q['code'] ?? '';
    final hosts = (q['ip'] ?? '').split(',').map((s) => s.trim()).where((s) => s.isNotEmpty).toList();
    if (id.isEmpty || !RegExp(r'^\d{6}$').hasMatch(code) || hosts.isEmpty) return null;
    return PairUri(
      machineId: id,
      name: q['name'] ?? 'Компьютер',
      os: q['os'] ?? '',
      port: int.tryParse(q['port'] ?? '') ?? defaultPort,
      code: code,
      hosts: hosts,
    );
  }
}

/// A paired computer as stored on the phone.
class SavedMachine {
  final String id;
  final String name;
  final String os;
  final int port;
  final List<String> hosts; // candidate addresses, best first
  final String token;
  final List<String> macs;
  final String? lastHost;
  final Map<String, dynamic>? lastState; // last `state` message, shown while offline
  final String? pinnedProfile; // profile chosen on the phone; null = follow the PC

  const SavedMachine({
    required this.id,
    required this.name,
    required this.os,
    required this.port,
    required this.hosts,
    required this.token,
    this.macs = const [],
    this.lastHost,
    this.lastState,
    this.pinnedProfile,
  });

  /// [pinnedProfile] uses a record so that "unpin" (null) can be told from "unchanged".
  SavedMachine copyWith({
    String? name,
    String? os,
    List<String>? hosts,
    String? token,
    List<String>? macs,
    String? lastHost,
    Map<String, dynamic>? lastState,
    (String?,)? pinnedProfile,
  }) => SavedMachine(
    id: id,
    name: name ?? this.name,
    os: os ?? this.os,
    port: port,
    hosts: hosts ?? this.hosts,
    token: token ?? this.token,
    macs: macs ?? this.macs,
    lastHost: lastHost ?? this.lastHost,
    lastState: lastState ?? this.lastState,
    pinnedProfile: pinnedProfile == null ? this.pinnedProfile : pinnedProfile.$1,
  );

  /// Addresses to try, the last one that worked first.
  List<String> get candidates => [
    ?lastHost,
    for (final h in hosts)
      if (h != lastHost) h,
  ];

  factory SavedMachine.fromJson(Map<String, dynamic> j) => SavedMachine(
    id: j['id'] as String,
    name: j['name'] as String? ?? '',
    os: j['os'] as String? ?? '',
    port: (j['port'] as num?)?.toInt() ?? defaultPort,
    hosts: ((j['hosts'] as List?) ?? const []).cast<String>(),
    token: j['token'] as String,
    macs: ((j['macs'] as List?) ?? const []).cast<String>(),
    lastHost: j['lastHost'] as String?,
    lastState: (j['lastState'] as Map?)?.cast<String, dynamic>(),
    pinnedProfile: j['pinnedProfile'] as String?,
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'os': os,
    'port': port,
    'hosts': hosts,
    'token': token,
    'macs': macs,
    if (lastHost != null) 'lastHost': lastHost,
    if (lastState != null) 'lastState': lastState,
    if (pinnedProfile != null) 'pinnedProfile': pinnedProfile,
  };
}
