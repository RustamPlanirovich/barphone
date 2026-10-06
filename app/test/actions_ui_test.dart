import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// A link that answers volume requests locally and records launches.
class FakeLink extends MachineLink {
  FakeLink()
    : super(
        const SavedMachine(id: 'm', name: 'Рабочий ПК', os: 'windows', port: 1, hosts: ['127.0.0.1'], token: 't'),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );

  double level = .40;
  final sets = <double>[];
  final launches = <String>[];
  String? answerError; // e.g. "busy": the next launches fail with it

  @override
  Future<LaunchResult> volume(String buttonId, [double? value]) async {
    if (value != null) {
      sets.add(value);
      level = value;
    }
    return LaunchResult.ok(action: 'volume', value: level, muted: false);
  }

  @override
  Future<LaunchResult> launch(String buttonId, {bool newInstance = false, bool confirmed = false}) async {
    launches.add('$buttonId${confirmed ? '+confirmed' : ''}');
    final err = answerError;
    return err == null ? const LaunchResult.ok(action: 'done') : LaunchResult.fail(err);
  }
}

Future<void> pumpTile(WidgetTester tester, DeckButton b, MachineLink link) => tester.pumpWidget(
  MaterialApp(
    theme: buildTheme(),
    home: Scaffold(
      body: Center(
        child: DeckTile(button: b, link: link, size: 140, enabled: true),
      ),
    ),
  ),
);

void main() {
  testWidgets('keys and system buttons show built-in glyphs', (tester) async {
    final link = FakeLink();
    await pumpTile(tester, const DeckButton(id: 'k', title: 'Ctrl+Shift+P', kind: 'keys', glyph: 'keys'), link);
    expect(find.byIcon(Icons.keyboard_rounded), findsOneWidget);
    await pumpTile(tester, const DeckButton(id: 'p', title: 'Пауза', kind: 'system', glyph: 'media_play_pause'), link);
    expect(find.byIcon(Icons.play_arrow_rounded), findsOneWidget);
  });

  testWidgets('a macro is one press; a second one while it runs says so', (tester) async {
    final link = FakeLink();
    await pumpTile(tester, const DeckButton(id: 'm1', title: 'Начать работу', kind: 'macro', glyph: 'macro'), link);
    expect(find.byIcon(Icons.bolt_rounded), findsOneWidget);
    await tester.tap(find.text('Начать работу'));
    await tester.pumpAndSettle();
    expect(link.launches, ['m1']);
    link.answerError = 'busy';
    await tester.tap(find.text('Начать работу'));
    await tester.pump();
    expect(find.text('Начать работу: Ещё выполняется другой макрос'), findsOneWidget);
  });

  testWidgets('shutdown asks first; cancel sends nothing, confirm sends "confirmed"', (tester) async {
    final link = FakeLink();
    const b = DeckButton(id: 'off', title: 'Выключить', kind: 'system', glyph: 'shutdown', confirm: true);
    await pumpTile(tester, b, link);
    await tester.tap(find.byType(DeckTile));
    await tester.pumpAndSettle();
    expect(find.text('Выключить?'), findsOneWidget);
    expect(find.text('Компьютер «Рабочий ПК» выполнит это сразу.'), findsOneWidget);
    await tester.tap(find.text('Отмена'));
    await tester.pumpAndSettle();
    expect(link.launches, isEmpty);

    await tester.tap(find.byType(DeckTile));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Выключить'));
    await tester.pumpAndSettle();
    expect(link.launches, ['off+confirmed']);
  });

  testWidgets('hold and drag the volume button sets the PC volume', (tester) async {
    final link = FakeLink();
    const b = DeckButton(id: 'vol', title: 'Громкость', kind: 'system', glyph: 'volume', control: 'slider');
    await pumpTile(tester, b, link);
    final g = await tester.startGesture(tester.getCenter(find.byType(DeckTile)));
    await tester.pump(kLongPressTimeout + const Duration(milliseconds: 50));
    await tester.pump();
    expect(find.text('40%'), findsOneWidget, reason: 'panel shows the current level');

    await g.moveBy(const Offset(65, 0)); // +65 px of 260 → +25 %
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('65%'), findsOneWidget);
    await g.moveBy(const Offset(0, 520)); // far down: clamps at 0
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('0%'), findsOneWidget);
    await g.up();
    await tester.pump(const Duration(milliseconds: 100));
    expect(link.sets, isNotEmpty);
    expect(link.sets.last, 0);
    expect(link.launches, isEmpty, reason: 'a drag is not a tap');
    await tester.pump(const Duration(seconds: 1));
    expect(find.text('0%'), findsNothing, reason: 'panel goes away after release');
  });

  testWidgets('tap on the volume button toggles mute and says so', (tester) async {
    final link = _MuteLink();
    const b = DeckButton(id: 'vol', title: 'Громкость', kind: 'system', glyph: 'volume', control: 'slider');
    await pumpTile(tester, b, link);
    await tester.tap(find.byType(DeckTile));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Звук выключен'), findsOneWidget);
  });
}

class _MuteLink extends FakeLink {
  @override
  Future<LaunchResult> launch(String buttonId, {bool newInstance = false, bool confirmed = false}) async =>
      const LaunchResult.ok(action: 'done', value: .4, muted: true);
}
