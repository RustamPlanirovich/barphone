import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/window_chooser.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  // A link that was never started: every request answers "offline", no network needed.
  final link = MachineLink(
    const SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: 1, hosts: ['127.0.0.1'], token: 't'),
    onChanged: () {},
    onMachineUpdated: (_) {},
  );
  const button = DeckButton(id: 'b1', title: 'VS Code', kind: 'app');
  const windows = [AppWindow('11', 'barphone'), AppWindow('12', 'AOv5'), AppWindow('13', 'ecobox')];

  Future<void> open(WidgetTester tester, List<AppWindow> ws, List<bool> flashes) async {
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

  testWidgets('lists the open windows and offers a new instance', (tester) async {
    final flashes = <bool>[];
    await open(tester, windows, flashes);
    expect(find.text('VS Code'), findsOneWidget);
    expect(find.text('Уже открыто окон: 3. Куда переключиться?'), findsOneWidget);
    for (final w in windows) {
      expect(find.text(w.title), findsOneWidget);
    }
    expect(find.text('Запустить новый'), findsOneWidget);

    // Picking a window sends focus; the unstarted link answers "offline".
    await tester.tap(find.text('AOv5'));
    await tester.pumpAndSettle();
    expect(find.text('Запустить новый'), findsNothing, reason: 'sheet closes');
    expect(flashes, [false]);
    expect(find.text('VS Code: Нет связи с компьютером'), findsOneWidget);
  });

  testWidgets('long press with no windows still offers a new instance', (tester) async {
    final flashes = <bool>[];
    await open(tester, const [], flashes);
    expect(find.text('Открытых окон нет'), findsOneWidget);
    await tester.tap(find.text('Запустить новый'));
    await tester.pumpAndSettle();
    expect(flashes, [false]);
  });
}
