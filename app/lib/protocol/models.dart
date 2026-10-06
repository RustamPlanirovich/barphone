// Wire and storage models for the barphone protocol (see docs/protocol.md).
// Pure Dart: no Flutter imports, so it can be tested on the host.

const int defaultPort = 47800;
const int protocolVersion = 1;

class MachineInfo {
  final String id;
  final String name;
  final String os;
  final List<String> macs;
  final List<String> addrs; // where the agent can be reached now, best first

  const MachineInfo({required this.id, required this.name, required this.os, this.macs = const [], this.addrs = const []});

  factory MachineInfo.fromJson(Map<String, dynamic> j) => MachineInfo(
    id: j['id'] as String,
    name: (j['name'] as String?) ?? '',
    os: (j['os'] as String?) ?? '',
    macs: ((j['macs'] as List?) ?? const []).cast<String>(),
    addrs: ((j['addrs'] as List?) ?? const []).cast<String>(),
  );
}

class DeckButton {
  final String id;
  final String title;
  final String kind; // app | path | url | keys | text | system | folder | macro | timer | stat | trackpad | command
  final String? icon;
  final String? glyph; // built-in picture: "keys", "text" or a system action id
  final String? control; // "slider": hold and drag (volume)
  final bool confirm; // ask before pressing (shutdown, restart)
  final List<DeckButton> buttons; // a folder's buttons
  final int? seconds; // a timer's duration
  final String? stat; // a live tile: "cpu" or "ram"

  const DeckButton({
    required this.id,
    required this.title,
    required this.kind,
    this.icon,
    this.glyph,
    this.control,
    this.confirm = false,
    this.buttons = const [],
    this.seconds,
    this.stat,
  });

  /// Starts a program on the PC, so it can have open windows to choose from.
  bool get launchesApp => kind == 'app' || kind == 'path';
  bool get isSlider => control == 'slider';

  /// Opens on the phone, showing its own buttons; the agent is not asked.
  bool get isFolder => kind == 'folder';

  /// Counts down [seconds] on the phone; the agent is not asked.
  bool get isTimer => kind == 'timer';

  /// Opens the trackpad and keyboard screen on the phone.
  bool get isTrackpad => kind == 'trackpad';

  /// Shows a number from the PC's "stats" messages.
  bool get isStat => kind == 'stat' && stat != null;

  factory DeckButton.fromJson(Map<String, dynamic> j) => DeckButton(
    id: j['id'] as String,
    title: (j['title'] as String?) ?? '',
    kind: (j['kind'] as String?) ?? 'app',
    icon: j['icon'] as String?,
    glyph: j['glyph'] as String?,
    control: j['control'] as String?,
    confirm: j['confirm'] == true,
    buttons: ((j['buttons'] as List?) ?? const []).map((b) => DeckButton.fromJson((b as Map).cast<String, dynamic>())).toList(),
    seconds: (j['seconds'] as num?)?.toInt(),
    stat: j['stat'] as String?,
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
    if (seconds != null) 'seconds': seconds,
    if (stat != null) 'stat': stat,
  };
}

/// A message from the PC ("notify"): a command finished, or a script said something.
class AgentNotice {
  final String machine; // name of the computer it came from
  final String title;
  final String text;
  final String level; // info | ok | error

  const AgentNotice({required this.machine, required this.title, required this.text, this.level = 'info'});

  factory AgentNotice.fromJson(String machine, Map<String, dynamic> j) => AgentNotice(
    machine: machine,
    title: (j['title'] as String?) ?? '',
    text: (j['text'] as String?) ?? '',
    level: (j['level'] as String?) ?? 'info',
  );
}

/// CPU load and memory of the PC, for live tiles; null fields were not measured.
class SysStats {
  final double? cpu; // 0..1
  final double? ram; // 0..1
  final int? ramUsed, ramTotal; // bytes

  const SysStats({this.cpu, this.ram, this.ramUsed, this.ramTotal});

  factory SysStats.fromJson(Map<String, dynamic> j) => SysStats(
    cpu: (j['cpu'] as num?)?.toDouble(),
    ram: (j['ram'] as num?)?.toDouble(),
    ramUsed: (j['ramUsed'] as num?)?.toInt(),
    ramTotal: (j['ramTotal'] as num?)?.toInt(),
  );

  double? of(String stat) => switch (stat) {
    'cpu' => cpu,
    'ram' => ram,
    _ => null,
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
  final String? tlsFp; // the agent's certificate fingerprint, for phones paired without it

  const DeckState({required this.machine, required this.profiles, required this.activeProfile, required this.recent, this.tlsFp});

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
      tlsFp: ((j['tls'] as Map?)?['fp']) as String?,
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
  final String? fp; // the agent's certificate fingerprint: pair over TLS

  const PairUri({
    required this.machineId,
    required this.name,
    required this.os,
    required this.port,
    required this.code,
    required this.hosts,
    this.fp,
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
      fp: (q['fp'] ?? '').isEmpty ? null : q['fp'],
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
  final String? fp; // the agent's certificate fingerprint (TLS); null = plain only
  final bool tlsOk; // TLS with [fp] has worked once: never fall back to plain again

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
    this.fp,
    this.tlsOk = false,
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
    String? fp,
    bool? tlsOk,
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
    fp: fp ?? this.fp,
    tlsOk: tlsOk ?? this.tlsOk,
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
    fp: j['fp'] as String?,
    tlsOk: j['tlsOk'] == true,
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
    if (fp != null) 'fp': fp,
    if (tlsOk) 'tlsOk': true,
  };
}
