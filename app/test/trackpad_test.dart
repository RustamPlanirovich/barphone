import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/trackpad.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Records what the trackpad screen sends.
class _Link extends MachineLink {
  _Link()
    : super(
        const SavedMachine(id: 'm', name: 'Рабочий ПК', os: 'windows', port: 1, hosts: ['127.0.0.1'], token: 't'),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
  final moves = <Offset>[];
  final wheel = <int>[];
  final sent = <String>[];
  String? fail;

  Offset get moved => moves.fold(Offset.zero, (a, b) => a + b);

  @override
  void pointer(String buttonId, int dx, int dy) => moves.add(Offset(dx.toDouble(), dy.toDouble()));

  @override
  void scroll(String buttonId, int dx, int dy) => wheel.add(dy);

  Future<LaunchResult> _ok(String what) async {
    sent.add(what);
    final f = fail;
    return f == null ? const LaunchResult.ok(action: 'done') : LaunchResult.fail(f);
  }

  @override
  Future<LaunchResult> click(String buttonId, String button, {bool double = false}) => _ok('click $button');

  @override
  Future<LaunchResult> typeText(String buttonId, String text) => _ok('type $text');

  @override
  Future<LaunchResult> key(String buttonId, String key) => _ok('key $key');
}

void main() {
  const pad = DeckButton(id: 'tp', title: 'Трекпад', kind: 'trackpad', glyph: 'trackpad');

  Future<_Link> open(WidgetTester tester) async {
    final link = _Link();
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: TrackpadScreen(link: link, button: pad),
      ),
    );
    return link;
  }

  testWidgets('one finger moves the cursor (faster = farther), a tap clicks', (tester) async {
    final link = await open(tester);
    final center = tester.getCenter(find.byKey(const ValueKey('pad')));
    final g = await tester.startGesture(center);
    for (var i = 0; i < 10; i++) {
      await g.moveBy(const Offset(4, 0));
      await tester.pump(const Duration(milliseconds: 20));
    }
    await g.up();
    await tester.pump(const Duration(milliseconds: 50));
    expect(link.moved.dx, greaterThan(40 * 1.6 - 1), reason: 'slow drag, gain ≥ 1.6');
    expect(link.moved.dy, 0);
    expect(link.sent, isEmpty, reason: 'a drag is not a click');

    final slow = link.moved.dx;
    link.moves.clear();
    final fast = await tester.startGesture(center);
    await fast.moveBy(const Offset(40, 0));
    await tester.pump(const Duration(milliseconds: 20));
    await fast.up();
    await tester.pump(const Duration(milliseconds: 50));
    expect(link.moved.dx, greaterThan(slow), reason: 'the same 40 px in one flick go farther');

    await tester.tapAt(center);
    await tester.pump();
    expect(link.sent, ['click left']);
  });

  testWidgets('two fingers scroll; a two-finger tap is a right click', (tester) async {
    final link = await open(tester);
    final c = tester.getCenter(find.byKey(const ValueKey('pad')));
    final a = await tester.startGesture(c - const Offset(30, 0), pointer: 1);
    final b = await tester.startGesture(c + const Offset(30, 0), pointer: 2);
    for (var i = 0; i < 6; i++) {
      await a.moveBy(const Offset(0, -10));
      await b.moveBy(const Offset(0, -10));
      await tester.pump(const Duration(milliseconds: 20));
    }
    await a.up();
    await b.up();
    await tester.pump(const Duration(milliseconds: 50));
    expect(link.wheel.fold(0, (s, v) => s + v), 240, reason: 'fingers up 60 px: two notches, natural scrolling');
    expect(link.moves, isEmpty, reason: 'scrolling does not move the cursor');

    final x = await tester.startGesture(c - const Offset(30, 0), pointer: 3);
    final y = await tester.startGesture(c + const Offset(30, 0), pointer: 4);
    await x.up();
    await y.up();
    await tester.pump();
    expect(link.sent, ['click right']);
  });

  testWidgets('the phone keyboard types into the PC; backspace and special keys', (tester) async {
    final link = await open(tester);
    await tester.tap(find.byTooltip('Клавиатура'));
    await tester.pump();
    expect(find.text('Esc'), findsOneWidget);
    final field = find.byKey(const ValueKey('keyboard'));
    await tester.enterText(field, '\u200Bпривет');
    await tester.enterText(field, '');
    await tester.tap(find.text('Esc'));
    await tester.testTextInput.receiveAction(TextInputAction.send);
    await tester.pump();
    expect(link.sent, ['type привет', 'key Backspace', 'key Escape', 'key Enter']);
    expect(tester.widget<TextField>(field).controller!.text, '\u200B', reason: 'ready for the next backspace');
  });

  testWidgets('says once why input does not arrive', (tester) async {
    final link = await open(tester);
    link.fail = 'unsupported';
    await tester.tap(find.text('Левая'));
    await tester.pump();
    expect(find.text('Этот компьютер так не умеет'), findsOneWidget);
    await tester.pump(const Duration(seconds: 1)); // shown...
    await tester.pump(const Duration(seconds: 4)); // ...and gone
    await tester.pumpAndSettle();
    expect(find.text('Этот компьютер так не умеет'), findsNothing);
    await tester.tap(find.text('Правая'));
    await tester.pump();
    expect(find.text('Этот компьютер так не умеет'), findsNothing);
  });
}
