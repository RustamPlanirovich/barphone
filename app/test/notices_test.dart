import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/home.dart';
import 'package:barphone/ui/theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class _App extends AppState {
  _App(this.link);
  final MachineLink link;

  @override
  MachineLink? get active => link;

  @override
  MachineLink? get companion => null;
}

void main() {
  testWidgets('a notice shows over the deck, goes away on a tap or by itself', (tester) async {
    final link = MachineLink(
      SavedMachine(
        id: 'm',
        name: 'Рабочий ПК',
        os: 'windows',
        port: 1,
        hosts: const ['127.0.0.1'],
        token: 't',
        lastState: {
          'type': 'state',
          'machine': {'id': 'm', 'name': 'Рабочий ПК', 'os': 'windows'},
          'deck': {
            'columns': 3,
            'buttons': [
              {'id': 'c', 'title': 'Сборка', 'kind': 'command', 'glyph': 'command', 'confirm': true},
            ],
          },
          'recent': <dynamic>[],
        },
      ),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    link.status = LinkStatus.online;
    final app = _App(link);
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: HomeScreen(app: app),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byIcon(Icons.terminal_rounded), findsOneWidget);
    final tileTop = tester.getTopLeft(find.text('Сборка'));

    app.notice(const AgentNotice(machine: 'Рабочий ПК', title: 'Сборка', text: 'Ошибка (код 3)', level: 'error'));
    await tester.pump(); // the stream delivers...
    await tester.pump(); // ...and the card is drawn
    expect(find.text('Рабочий ПК · Сборка'), findsOneWidget);
    expect(find.text('Ошибка (код 3)'), findsOneWidget);
    expect(find.byIcon(Icons.error_rounded), findsOneWidget);
    expect(tester.getTopLeft(find.text('Сборка').first), tileTop, reason: 'the deck does not move');
    await tester.tap(find.text('Ошибка (код 3)'));
    await tester.pump();
    expect(find.text('Ошибка (код 3)'), findsNothing);

    app.notice(const AgentNotice(machine: 'Рабочий ПК', title: 'Деплой', text: 'Готово', level: 'ok'));
    await tester.pump();
    await tester.pump();
    expect(find.byIcon(Icons.check_circle_rounded), findsOneWidget);
    await tester.pump(const Duration(seconds: 7));
    expect(find.text('Готово'), findsNothing);
  });
}
