import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/deck_page.dart';
import 'package:barphone/ui/home.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

Map<String, dynamic> stateWith(List<Map<String, dynamic>> buttons) => {
  'type': 'state',
  'machine': {'id': 'm', 'name': 'ПК', 'os': 'windows'},
  'deck': {'id': 'default', 'name': 'Основной', 'columns': 3, 'buttons': buttons},
  'recent': [
    {'id': 'steam', 'at': '2026-10-06T10:00:00Z'},
  ],
};

const games = {
  'id': 'games',
  'title': 'Игры',
  'kind': 'folder',
  'glyph': 'folder',
  'buttons': [
    {'id': 'steam', 'title': 'Steam', 'kind': 'url'},
    {'id': 'gog', 'title': 'GOG', 'kind': 'url'},
  ],
};
const mail = {'id': 'mail', 'title': 'Почта', 'kind': 'url'};

class _App extends AppState {
  _App(this.link);
  final MachineLink link;

  @override
  MachineLink? get active => link;

  @override
  MachineLink? get companion => null;
}

void main() {
  test('buttons inside folders are found (recent launches)', () {
    final st = DeckState.fromJson(stateWith([mail, games]));
    expect(st.button('steam')?.title, 'Steam');
    expect(st.buttons[1].isFolder, isTrue);
    expect(st.buttons[1].buttons.map((b) => b.id), ['steam', 'gog']);
  });

  testWidgets('a folder opens on the phone; back tile and system back close it', (tester) async {
    final link = MachineLink(
      SavedMachine(
        id: 'm',
        name: 'ПК',
        os: 'windows',
        port: 1,
        hosts: const ['127.0.0.1'],
        token: 't',
        lastState: stateWith([mail, games]),
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

    expect(find.text('Почта'), findsOneWidget);
    expect(find.text('Steam'), findsNothing);
    await tester.tap(find.text('Игры'));
    await tester.pumpAndSettle();
    expect(find.text('Почта'), findsNothing);
    expect(find.text('Steam'), findsOneWidget);
    expect(find.text('GOG'), findsOneWidget);
    expect(find.byType(BackTile), findsOneWidget);

    await tester.tap(find.byType(BackTile));
    await tester.pumpAndSettle();
    expect(find.text('Почта'), findsOneWidget);
    expect(find.byType(BackTile), findsNothing);

    // System back (gesture or button) closes the folder and stays on the deck.
    await tester.tap(find.text('Игры'));
    await tester.pumpAndSettle();
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.text('Почта'), findsOneWidget);
    expect(find.byType(DeckPage), findsOneWidget);
    expect(find.text('недавние'), findsOneWidget, reason: 'still on the deck page');

    // The folder disappears on the PC while open: back to the top level.
    await tester.tap(find.text('Игры'));
    await tester.pumpAndSettle();
    link.state = DeckState.fromJson(stateWith([mail]));
    app.notifyListeners();
    await tester.pumpAndSettle();
    expect(find.text('Почта'), findsOneWidget);
    expect(find.byType(BackTile), findsNothing);
  });

  testWidgets('a folder opens even while the computer is away', (tester) async {
    final link = MachineLink(
      SavedMachine(
        id: 'm',
        name: 'ПК',
        os: 'windows',
        port: 1,
        hosts: const ['127.0.0.1'],
        token: 't',
        lastState: stateWith([mail, games]),
      ),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    link.status = LinkStatus.offline;
    final app = _App(link);
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: DeckPage(app: app, link: link, onShowMachines: () {}, onAddMachine: () {}),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Игры'));
    await tester.pumpAndSettle();
    expect(find.text('Steam'), findsOneWidget);
    expect(tester.widget<DeckTile>(find.widgetWithText(DeckTile, 'Steam')).enabled, isFalse);
  });
}
