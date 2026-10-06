import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/window_chooser.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Records what the chooser asks the PC to do; [fail] makes every request answer "offline".
class _Link extends MachineLink {
  _Link({this.fail = false})
    : super(
        const SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: 1, hosts: ['127.0.0.1'], token: 't'),
        onChanged: () {},
        onMachineUpdated: (_) {},
      );
  final bool fail;
  final calls = <String>[];

  Future<LaunchResult> _answer(String call, String action) async {
    calls.add(call);
    return fail ? const LaunchResult.fail('offline') : LaunchResult.ok(action: action);
  }

  @override
  Future<LaunchResult> focus(String buttonId, String windowId) => _answer('focus $windowId', 'focused');

  @override
  Future<LaunchResult> minimize(String buttonId, [String? windowId]) => _answer('minimize ${windowId ?? 'all'}', 'minimized');

  @override
  Future<LaunchResult> launch(String buttonId, {bool newInstance = false, bool confirmed = false}) =>
      _answer(newInstance ? 'new' : 'launch', 'launched');
}

void main() {
  const button = DeckButton(id: 'b1', title: 'VS Code', kind: 'app');
  const windows = [AppWindow('11', 'barphone — Visual Studio Code', active: true), AppWindow('12', 'AOv5'), AppWindow('13', 'ecobox')];

  Future<void> open(WidgetTester tester, MachineLink link, List<AppWindow> ws, List<bool> flashes) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: Builder(
            builder: (context) => Center(
              child: TextButton(
                onPressed: () => showWindowChooser(context, link, button, ws, flash: flashes.add),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
  }

  testWidgets('windows are tiles; the one in front is marked and minimizes', (tester) async {
    final link = _Link();
    final flashes = <bool>[];
    await open(tester, link, windows, flashes);
    expect(find.text('VS Code'), findsOneWidget);
    expect(find.text('Открыто окон: 3'), findsOneWidget);
    for (final w in windows) {
      expect(find.text(w.title), findsOneWidget);
    }
    expect(find.byType(GridView), findsOneWidget);
    expect(find.text('впереди'), findsOneWidget);
    expect(find.text('тап — свернуть'), findsOneWidget);
    expect(find.text('тап — перейти'), findsNWidgets(2));
    expect(find.text('Запустить новый'), findsOneWidget);
    expect(find.text('Свернуть все'), findsOneWidget);

    await tester.tap(find.text('barphone — Visual Studio Code'));
    await tester.pumpAndSettle();
    expect(find.text('Запустить новый'), findsNothing, reason: 'sheet closes');
    expect(link.calls, ['minimize 11']);
    expect(flashes, [true]);

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('AOv5'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Свернуть все'));
    await tester.pumpAndSettle();
    expect(link.calls, ['minimize 11', 'focus 12', 'minimize all']);
  });

  testWidgets('a failed request flashes red and says why', (tester) async {
    final link = _Link(fail: true);
    final flashes = <bool>[];
    await open(tester, link, windows, flashes);
    await tester.tap(find.text('ecobox'));
    await tester.pumpAndSettle();
    expect(link.calls, ['focus 13']);
    expect(flashes, [false]);
    expect(find.text('VS Code: Нет связи с компьютером'), findsOneWidget);
  });

  testWidgets('long press with no windows still offers a new instance', (tester) async {
    final link = _Link();
    final flashes = <bool>[];
    await open(tester, link, const [], flashes);
    expect(find.text('Открытых окон нет'), findsOneWidget);
    expect(find.text('Свернуть все'), findsNothing);
    await tester.tap(find.text('Запустить новый'));
    await tester.pumpAndSettle();
    expect(link.calls, ['new']);
    expect(flashes, [true]);
  });
}
