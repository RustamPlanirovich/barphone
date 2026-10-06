import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/timers.dart';
import 'package:barphone/ui/home.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

const pomodoro = DeckButton(id: 't1', title: 'Pomodoro', kind: 'timer', glyph: 'timer', seconds: 25 * 60);

MachineLink link({Map<String, dynamic>? state}) => MachineLink(
  SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: 1, hosts: const ['127.0.0.1'], token: 't', lastState: state),
  onChanged: () {},
  onMachineUpdated: (_) {},
);

class _App extends AppState {
  _App(this.link, DeckTimers timers) : super(timers: timers);
  final MachineLink link;

  @override
  MachineLink? get active => link;

  @override
  MachineLink? get companion => null;
}

void main() {
  test('formatLeft', () {
    expect(formatLeft(const Duration(minutes: 25)), '25:00');
    expect(formatLeft(const Duration(seconds: 61, milliseconds: 200)), '1:02');
    expect(formatLeft(const Duration(hours: 1, minutes: 5)), '1:05:00');
    expect(formatLeft(Duration.zero), '0:00');
  });

  test('a timer ends on the wall clock and says so once', () async {
    var now = DateTime(2026, 10, 6, 12);
    final timers = DeckTimers(now: () => now);
    final done = <String>[];
    final sub = timers.finished.listen(done.add);
    final key = DeckTimers.key('m', 't1');
    timers.start(key, const Duration(minutes: 25), 'Pomodoro');
    expect(timers.left(key), const Duration(minutes: 25));
    now = now.add(const Duration(minutes: 10));
    timers.check();
    expect(timers.left(key), const Duration(minutes: 15));
    now = now.add(const Duration(minutes: 20)); // e.g. the app was in the background
    timers.check();
    timers.check();
    await Future<void>.delayed(Duration.zero);
    expect(done, ['Pomodoro']);
    expect(timers.running(key), isFalse);
    expect(DeckTimers.key('a', 'x'), isNot(DeckTimers.key('b', 'x')), reason: 'two computers, same button id');
    await sub.cancel();
    timers.dispose();
  });

  testWidgets('tap starts the countdown; tap again asks before stopping', (tester) async {
    var now = DateTime(2026, 10, 6, 12);
    final timers = DeckTimers(now: () => now);
    final l = link();
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: Center(
            child: DeckTile(button: pomodoro, link: l, size: 140, enabled: true, timers: timers),
          ),
        ),
      ),
    );
    expect(find.byIcon(Icons.timer_outlined), findsOneWidget);
    await tester.tap(find.byType(DeckTile));
    await tester.pump();
    expect(find.text('25:00'), findsOneWidget);
    now = now.add(const Duration(minutes: 1));
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.text('24:00'), findsOneWidget);

    await tester.tap(find.byType(DeckTile));
    await tester.pumpAndSettle();
    expect(find.text('Осталось 24:00.'), findsOneWidget);
    await tester.tap(find.text('Пусть идёт'));
    await tester.pumpAndSettle();
    expect(timers.running(DeckTimers.key('m', 't1')), isTrue);

    await tester.tap(find.byType(DeckTile));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Остановить'));
    await tester.pumpAndSettle();
    expect(timers.running(DeckTimers.key('m', 't1')), isFalse);
    expect(find.byIcon(Icons.timer_outlined), findsOneWidget);
  });

  testWidgets('when the time is up the home screen buzzes and says so', (tester) async {
    var now = DateTime(2026, 10, 6, 12);
    final timers = DeckTimers(now: () => now);
    final l = link(
      state: {
        'type': 'state',
        'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
        'deck': {
          'id': 'default',
          'name': 'Основной',
          'columns': 3,
          'buttons': [pomodoro.toJson()],
        },
        'recent': <dynamic>[],
      },
    );
    l.status = LinkStatus.offline; // timers work without the computer
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: HomeScreen(app: _App(l, timers)),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Pomodoro'));
    await tester.pump();
    expect(find.text('25:00'), findsOneWidget);

    now = now.add(const Duration(minutes: 25, seconds: 1));
    await tester.pump(const Duration(milliseconds: 300)); // a tick
    await tester.pump();
    expect(find.text('Время вышло'), findsOneWidget);
    await tester.pump(const Duration(seconds: 2)); // the buzzing
    await tester.tap(find.text('OK'));
    await tester.pumpAndSettle();
    expect(find.text('Время вышло'), findsNothing);
    expect(timers.running(DeckTimers.key('m', 't1')), isFalse);
  });
}
