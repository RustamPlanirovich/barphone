import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/call_panel.dart';
import 'package:barphone/ui/deck_page.dart';
import 'package:barphone/ui/home.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:barphone/ui/window_chooser.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class _Link extends MachineLink {
  _Link(String id, String name)
    : super(
        SavedMachine(
          id: id,
          name: name,
          os: 'windows',
          port: 1,
          hosts: const ['127.0.0.1'],
          token: 't',
          lastState: {
            'type': 'state',
            'machine': {'id': id, 'name': name, 'os': 'windows'},
            'deck': {
              'columns': 2,
              'buttons': [
                {'id': '$id-1', 'title': 'Почта $name', 'kind': 'url'},
              ],
            },
            'recent': <dynamic>[],
          },
        ),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
  final actions = <String>[];
  String? fail;

  @override
  Future<LaunchResult> callAction(String action) async {
    actions.add(action);
    final f = fail;
    return f == null ? const LaunchResult.ok(action: 'call') : LaunchResult.fail(f);
  }
}

class _App extends AppState {
  _App(this.first, [this.second]);
  final MachineLink first;
  final MachineLink? second;

  @override
  MachineLink? get active => first;

  @override
  MachineLink? get companion => second;
}

void main() {
  Future<void> show(WidgetTester tester, AppState app, Size size) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = size;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: HomeScreen(app: app),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('one computer in a Meet call: a strip one tile wide next to the deck, five controls', (tester) async {
    final pc = _Link('m', 'ПК')..status = LinkStatus.online;
    await show(tester, _App(pc), const Size(890, 410));
    expect(find.byType(CallPanel), findsNothing);
    expect(find.byType(DeckPage), findsOneWidget);

    pc.call.value = const CallInfo(active: true, camera: true);
    await tester.pumpAndSettle();
    expect(find.byType(CallPanel), findsOneWidget);
    final strip = tester.getRect(find.byType(CallPanel));
    final tile = tester.getRect(find.byType(DeckTile).first);
    expect(strip.width, closeTo(tile.width + 16, 10), reason: 'one tile wide (the deck may shrink its tiles a little to fit)');
    expect(strip.right, closeTo(890, 1));
    expect(tester.getCenter(find.text('Почта ПК')).dx, lessThan(strip.left), reason: 'the deck keeps the rest');
    final labels = ['Микрофон', 'Камера вкл', 'Рука', 'Открыть', 'Выйти'];
    final ys = [for (final l in labels) tester.getCenter(find.text(l)).dy];
    expect(ys, orderedEquals([...ys]..sort()), reason: 'stacked top to bottom');

    await tester.tap(find.text('Камера вкл'));
    await tester.tap(find.text('Микрофон'));
    await tester.tap(find.text('Рука'));
    await tester.tap(find.text('Открыть'));
    await tester.pump();
    expect(pc.actions, ['camera', 'mic', 'hand', 'show']);

    // Leaving asks first.
    await tester.tap(find.text('Выйти'));
    await tester.pumpAndSettle();
    expect(find.text('Выйти из звонка?'), findsOneWidget);
    await tester.tap(find.text('Остаться'));
    await tester.pumpAndSettle();
    expect(pc.actions, hasLength(4));
    await tester.tap(find.text('Выйти'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Выйти'));
    await tester.pumpAndSettle();
    expect(pc.actions.last, 'leave');

    pc.call.value = const CallInfo(active: true, camera: false);
    await tester.pump();
    expect(find.text('Камера выкл'), findsOneWidget);

    pc.fail = 'not_found';
    await tester.tap(find.text('Микрофон'));
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('Окно Meet не найдено'), findsOneWidget);

    pc.call.value = null; // the call ended
    await tester.pumpAndSettle();
    expect(find.byType(CallPanel), findsNothing);
  });

  testWidgets('no call panel in portrait or with a second computer', (tester) async {
    final pc = _Link('m', 'ПК')..status = LinkStatus.online;
    pc.call.value = const CallInfo(active: true, camera: true);
    await show(tester, _App(pc), const Size(410, 890));
    expect(find.byType(CallPanel), findsNothing);

    final second = _Link('w', 'Ноутбук')..status = LinkStatus.online;
    await show(tester, _App(pc, second), const Size(890, 410));
    expect(find.byType(CallPanel), findsNothing);
    expect(find.text('Почта Ноутбук'), findsOneWidget);
  });

  testWidgets('a window on another desktop says so in the chooser', (tester) async {
    final pc = _Link('m', 'ПК');
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () => showWindowChooser(context, pc, const DeckButton(id: 'b', title: 'VS Code', kind: 'app'), const [
                AppWindow('1', 'barphone', active: true),
                AppWindow('2', 'ecobox', desktop: 4),
              ], flash: (_) {}),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    expect(find.text('на столе 4 · тап — туда'), findsOneWidget);
    expect(find.text('тап — свернуть'), findsOneWidget);
  });
}
