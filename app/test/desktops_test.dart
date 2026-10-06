import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/deck_page.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/trackpad.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Records desktop requests and answers like the agent (no wrapping around).
class _Link extends MachineLink {
  _Link({Map<String, dynamic>? state})
    : super(
        SavedMachine(id: 'm', name: 'Рабочий ПК', os: 'windows', port: 1, hosts: const ['127.0.0.1'], token: 't', lastState: state),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
  final calls = <String>[];

  @override
  Future<LaunchResult> desktop(String buttonId, {int? move, int? to, bool overview = false}) async {
    calls.add(overview ? 'overview' : (move != null ? 'move $move' : 'to $to'));
    final d = desktops.value;
    if (d != null && move != null) desktops.value = d.moved(move);
    if (d != null && to != null) desktops.value = d.moved(to - d.current);
    return LaunchResult.ok(action: 'desktop', desktops: desktops.value);
  }

  @override
  Future<LaunchResult> launch(String buttonId, {bool newInstance = false, bool confirmed = false}) async {
    calls.add('launch $buttonId');
    return const LaunchResult.ok(action: 'done');
  }
}

Map<String, dynamic> deckState() => {
  'type': 'state',
  'machine': {'id': 'm', 'name': 'Рабочий ПК', 'os': 'windows'},
  'deck': {
    'id': 'default',
    'name': 'Основной',
    'columns': 3,
    'buttons': [
      {'id': 'vd', 'title': 'Рабочие столы', 'kind': 'system', 'glyph': 'desktops', 'control': 'desktops'},
      for (var i = 1; i < 30; i++) {'id': 'b$i', 'title': 'Кнопка $i', 'kind': 'url'},
    ],
  },
  'recent': <dynamic>[],
};

void main() {
  testWidgets('swipe on the desktops tile flips desktops, not the deck; elsewhere it pages the deck', (tester) async {
    final link = _Link(state: deckState())..status = LinkStatus.online;
    link.desktops.value = const DesktopInfo(count: 5, current: 1, names: ['', '', '', '', '']);
    final app = AppState();
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: DeckPage(app: app, link: link, onShowMachines: () {}, onAddMachine: () {}),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('2 / 5'), findsOneWidget);
    final tile = tester.getCenter(find.text('Рабочие столы'));

    await tester.dragFrom(tile, const Offset(-80, 0));
    await tester.pumpAndSettle();
    expect(link.calls, ['move 1'], reason: 'finger to the left = the next desktop');
    expect(find.text('3 / 5'), findsOneWidget);
    expect(find.text('Кнопка 1'), findsOneWidget, reason: 'the deck did not page');

    await tester.dragFrom(tile, const Offset(80, 0));
    await tester.pumpAndSettle();
    expect(link.calls.last, 'move -1');

    // At the first desktop a swipe further left only bumps (no request).
    link.desktops.value = const DesktopInfo(count: 5, current: 0, names: ['', '', '', '', '']);
    await tester.pump();
    await tester.dragFrom(tile, const Offset(80, 0));
    await tester.pumpAndSettle();
    expect(link.calls, hasLength(2));

    // A swipe on another tile pages the deck as before.
    await tester.flingFrom(tester.getCenter(find.text('Кнопка 4')), const Offset(-400, 0), 1500);
    await tester.pumpAndSettle();
    expect(find.text('Кнопка 1'), findsNothing);
    expect(link.calls, hasLength(2));
  });

  testWidgets('tap shows the overview; hold lists the desktops, a tap goes there', (tester) async {
    final link = _Link(state: deckState())..status = LinkStatus.online;
    link.desktops.value = const DesktopInfo(count: 4, current: 0, names: ['', 'Работа', '', '']);
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: DeckPage(app: AppState(), link: link, onShowMachines: () {}, onAddMachine: () {}),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Рабочие столы'));
    await tester.pumpAndSettle();
    expect(link.calls, ['launch vd']);

    await tester.longPress(find.text('Рабочие столы'));
    await tester.pumpAndSettle();
    expect(find.text('Рабочие столы · Рабочий ПК'), findsOneWidget);
    expect(find.text('Работа'), findsOneWidget);
    expect(find.text('Рабочий стол 3'), findsOneWidget);
    await tester.tap(find.text('Работа'));
    await tester.pumpAndSettle();
    expect(link.calls.last, 'to 1');
    expect(find.text('Работа'), findsOneWidget, reason: 'a named desktop shows its name on the tile');
  });

  testWidgets('trackpad: three fingers left/right flip desktops, up shows the overview', (tester) async {
    final link = _Link();
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: TrackpadScreen(
          link: link,
          button: const DeckButton(id: 'tp', title: 'Трекпад', kind: 'trackpad'),
        ),
      ),
    );
    final c = tester.getCenter(find.byKey(const ValueKey('pad')));
    Future<void> three(Offset by) async {
      final g = [for (var i = 0; i < 3; i++) await tester.startGesture(c + Offset(i * 30.0 - 30, 0), pointer: 10 + i)];
      for (var step = 0; step < 5; step++) {
        for (final f in g) {
          await f.moveBy(by / 5);
        }
        await tester.pump(const Duration(milliseconds: 16));
      }
      for (final f in g) {
        await f.up();
      }
      await tester.pump();
    }

    await three(const Offset(-120, 0));
    await three(const Offset(120, 0));
    await three(const Offset(0, -120));
    expect(link.calls, ['move 1', 'move -1', 'overview']);
  });
}
