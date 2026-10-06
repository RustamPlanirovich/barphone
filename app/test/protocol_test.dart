import 'package:barphone/protocol/layout.dart';
import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/protocol/wol.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('PairUri', () {
    test('parses what the agent puts into the QR code', () {
      final p = PairUri.parse(
        'barphone://pair?code=683304&id=5f0c2a9e41d7b3c86e0a19f2d4b7c5e1&ip=192.168.1.20%2C10.8.0.5&name=Work-PC&os=windows&port=47800&v=1',
      )!;
      expect(p.code, '683304');
      expect(p.hosts, ['192.168.1.20', '10.8.0.5']);
      expect(p.port, 47800);
      expect(p.name, 'Work-PC');
    });

    test('decodes cyrillic names', () {
      final p = PairUri.parse('barphone://pair?id=x&code=000001&ip=10.0.0.2&name=%D0%A0%D0%B0%D0%B1%D0%BE%D1%87%D0%B8%D0%B9')!;
      expect(p.name, 'Рабочий');
      expect(p.port, defaultPort);
    });

    test('rejects foreign or incomplete links', () {
      expect(PairUri.parse('https://example.com'), isNull);
      expect(PairUri.parse('barphone://pair?id=x&code=12&ip=1.2.3.4'), isNull);
      expect(PairUri.parse('barphone://pair?id=x&code=123456'), isNull);
      expect(PairUri.parse('barphone://other?id=x&code=123456&ip=1.2.3.4'), isNull);
    });
  });

  test('DeckState tolerates missing optional fields and resolves recents', () {
    final st = DeckState.fromJson({
      'type': 'state',
      'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
      'deck': {
        'columns': 4,
        'buttons': [
          {'id': 'a', 'title': 'A', 'kind': 'app', 'icon': null},
          {'id': 'b', 'title': 'B', 'kind': 'url', 'icon': '0123456789abcdef'},
        ],
      },
      'recent': [
        {'id': 'b', 'at': '2026-10-06T14:31:05Z'},
        {'id': 'gone', 'at': '2026-10-06T14:30:00Z'},
      ],
    });
    expect(st.columns, 4);
    expect(st.machine.macs, isEmpty);
    expect(st.buttons.first.icon, isNull);
    expect(st.recentButtons.map((r) => r.$1.id), ['b']);
  });

  test('SavedMachine round-trips and prefers the last working host', () {
    const m = SavedMachine(id: 'm', name: 'n', os: 'windows', port: 1, hosts: ['a', 'b'], token: 't', lastHost: 'b');
    final back = SavedMachine.fromJson(m.toJson());
    expect(back.candidates, ['b', 'a']);
    expect(back.token, 't');
  });

  group('grid', () {
    test('portrait phone keeps the configured columns', () {
      final g = computeGrid(400, 700, 3);
      expect(g.columns, 3);
      expect(g.rows, greaterThanOrEqualTo(4));
      expect(g.tile, closeTo((400 - 12 * 4) / 3, 0.001));
    });
    test('landscape: the configured count becomes rows, columns fill the width', () {
      final g = computeGrid(800, 360, 3);
      expect(g.rows, 3);
      expect(g.columns, greaterThan(3));
      expect(g.pages(20), (20 / g.perPage).ceil());
    });
    // A 360 dp phone in landscape with a 2-column deck: header, hints and safe area leave
    // about 790 x 287 for the grid; page dots take 18 more.
    test('page dots do not cost a row', () {
      final one = computeGrid(790, 287, 2);
      final many = computeGrid(790, 287 - 18, 2);
      expect([one.rows, one.columns], [2, 6]);
      expect([many.rows, many.columns], [2, 6]);
      expect(many.tile, greaterThan(100));
    });
    test('half of a split screen: same rows, half the columns', () {
      for (final h in [287.0, 269.0]) {
        final g = computeGrid(395, h, 2, landscape: true);
        expect([g.rows, g.columns], [2, 3], reason: 'height $h');
      }
      final half = computeGrid(395, 287, 2, landscape: true);
      expect(half.tile, closeTo(computeGrid(790, 287, 2).tile, 10));
    });
    test('landscape drops rows only when tiles would get tiny', () {
      expect(computeGrid(800, 300, 4).rows, 4);
      expect(computeGrid(800, 200, 4).rows, 2);
      expect(computeGrid(800, 100, 4).rows, 1);
    });
  });

  test('magic packet is 6x FF + 16x MAC', () {
    final mac = parseMac('a4:bb:6d:12:34:56')!;
    final p = magicPacket(mac);
    expect(p.length, 102);
    expect(p.sublist(0, 6), List.filled(6, 0xff));
    expect(p.sublist(96), mac);
    expect(parseMac('zz:bb:6d:12:34:56'), isNull);
  });

  group('LaunchResult', () {
    test('choose carries the windows', () {
      final r = LaunchResult.fromJson({
        'type': 'result',
        'req': 'r1',
        'ok': true,
        'action': 'choose',
        'windows': [
          {'id': '11', 'title': 'barphone'},
          {'id': '12', 'title': 'AOv5'},
        ],
      });
      expect(r.needsChoice, isTrue);
      expect(r.windows.map((w) => w.title), ['barphone', 'AOv5']);
    });
    test('launched / focused need no choice; old agents send no action', () {
      expect(LaunchResult.fromJson({'ok': true, 'action': 'focused'}).needsChoice, isFalse);
      expect(LaunchResult.fromJson({'ok': true}).needsChoice, isFalse);
      expect(LaunchResult.fromJson({'ok': true, 'action': 'windows', 'windows': []}).needsChoice, isTrue);
    });
    test('errors', () {
      final r = LaunchResult.fromJson({'ok': false, 'error': 'window_gone'});
      expect(r.ok, isFalse);
      expect(r.error, 'window_gone');
    });
    test('volume answers carry the level and mute state', () {
      final r = LaunchResult.fromJson({'ok': true, 'action': 'volume', 'value': 0.42, 'muted': false});
      expect(r.value, closeTo(.42, 1e-9));
      expect(r.muted, isFalse);
      expect(LaunchResult.fromJson({'ok': true, 'action': 'volume', 'value': 1}).value, 1.0, reason: 'ints are fine too');
    });
  });

  group('profiles', () {
    Map<String, dynamic> state({String active = 'vs', bool withProfiles = true}) => {
      'type': 'state',
      'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
      'deck': {
        'columns': 4,
        'buttons': [
          {'id': 'p1', 'title': 'Палитра', 'kind': 'keys', 'glyph': 'keys'},
        ],
      },
      if (withProfiles)
        'profiles': [
          {
            'id': 'default',
            'name': 'Основной',
            'columns': 3,
            'buttons': [
              {'id': 'a', 'title': 'A', 'kind': 'app'},
            ],
          },
          {
            'id': 'vs',
            'name': 'VS Code',
            'columns': 4,
            'buttons': [
              {'id': 'p1', 'title': 'Палитра', 'kind': 'keys', 'glyph': 'keys'},
            ],
          },
        ],
      if (withProfiles) 'activeProfile': active,
      'recent': [
        {'id': 'a', 'at': '2026-10-06T10:00:00Z'},
        {'id': 'p1', 'at': '2026-10-06T09:00:00Z'},
      ],
    };

    test('the PC picks the profile unless one is pinned; stale ids fall back', () {
      final st = DeckState.fromJson(state());
      expect(st.profiles.map((p) => p.id), ['default', 'vs']);
      expect(st.shown(null).id, 'vs');
      expect(st.shown('default').id, 'default');
      expect(st.shown('deleted').id, 'vs', reason: 'a pin to a deleted profile is ignored');
      expect(DeckState.fromJson(state(active: 'gone')).shown(null).id, 'default');
      expect(st.button('a')?.title, 'A', reason: 'buttons are found in any profile');
      expect(st.recentButtons.map((r) => r.$1.id), ['a', 'p1']);
    });

    test('an agent without profiles shows its single deck as the default profile', () {
      final st = DeckState.fromJson(state(withProfiles: false));
      expect(st.profiles.single.id, defaultProfileId);
      expect(st.shown(null).columns, 4);
      expect(st.buttons.single.id, 'p1');
    });

    test('the pin survives storage and copyWith; it can be cleared', () {
      const m = SavedMachine(id: 'm', name: 'n', os: 'windows', port: 1, hosts: ['a'], token: 't', pinnedProfile: 'vs');
      final back = SavedMachine.fromJson(m.toJson());
      expect(back.pinnedProfile, 'vs');
      expect(back.copyWith(lastHost: 'a').pinnedProfile, 'vs');
      expect(back.copyWith(pinnedProfile: (null,)).pinnedProfile, isNull);
    });
  });

  test('action buttons: glyph, slider and confirm flags; old states default safely', () {
    final b = DeckButton.fromJson({
      'id': 'v',
      'title': 'Громкость',
      'kind': 'system',
      'icon': null,
      'glyph': 'volume',
      'control': 'slider',
    });
    expect(b.isSlider, isTrue);
    expect(b.launchesApp, isFalse);
    expect(b.confirm, isFalse);
    final off = DeckButton.fromJson({'id': 'o', 'title': 'Выключить', 'kind': 'system', 'glyph': 'shutdown', 'confirm': true});
    expect(off.confirm, isTrue);
    final old = DeckButton.fromJson({'id': 'a', 'title': 'Code', 'kind': 'app'});
    expect(old.launchesApp, isTrue);
    expect(old.glyph, isNull);
    expect(DeckButton.fromJson(off.toJson()).confirm, isTrue, reason: 'cached state round-trips');
  });
}
