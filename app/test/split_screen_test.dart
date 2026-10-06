import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/deck_page.dart';
import 'package:barphone/ui/home.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

MachineLink machine(String id, String name, List<String> titles) {
  final link = MachineLink(
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
          'id': 'default',
          'name': 'Основной',
          'columns': 2, // as on the PC this was reported from
          'buttons': [
            for (final (i, t) in titles.indexed) {'id': '$id$i', 'title': t, 'kind': 'url'},
          ],
        },
        'recent': <dynamic>[],
      },
    ),
    onChanged: () {},
    onMachineUpdated: (_) {},
  );
  link.status = LinkStatus.online;
  return link;
}

/// Two computers without storage or network.
class _App extends AppState {
  _App(this.first, this.second);
  final MachineLink first;
  MachineLink? second;

  @override
  MachineLink? get active => first;

  @override
  MachineLink? get companion => second;
}

void main() {
  final home = machine('h', 'Домашний', ['Почта', for (var i = 2; i < 30; i++) 'Кнопка $i', 'Последняя']);
  final work = machine('w', 'Рабочий', ['Jira', 'Git', 'CI']);

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

  double centerX(WidgetTester tester, String title) => tester.getCenter(find.text(title)).dx;

  List<Rect> tiles(WidgetTester tester, Finder page) => [
    for (final e in find.descendant(of: page, matching: find.byType(DeckTile)).evaluate()) tester.getRect(find.byWidget(e.widget)),
  ];
  int rowsOf(List<Rect> rects) => {for (final r in rects) r.top}.length;

  testWidgets('landscape with two computers: two decks side by side, extra tiles page sideways', (tester) async {
    await show(tester, _App(home, work), const Size(890, 410));
    expect(find.byType(DeckPage), findsNWidgets(2));
    expect(find.text('Домашний'), findsOneWidget);
    expect(find.text('Рабочий'), findsOneWidget);
    expect(centerX(tester, 'Домашний'), lessThan(445), reason: 'active computer on the left');
    expect(centerX(tester, 'Рабочий'), greaterThan(445));
    expect(centerX(tester, 'Почта'), lessThan(445));
    expect(centerX(tester, 'Jira'), greaterThan(445));
    expect(find.text('компьютеры'), findsOneWidget, reason: 'swipe hints are shared');
    expect(find.text('недавние'), findsOneWidget);

    // A 2-column deck shows 2 rows in landscape, in each half too (even with page dots);
    // the rest of the left deck is on further pages, swiped sideways within its half.
    final left = tiles(tester, find.byType(DeckPage).first);
    expect(left.every((r) => r.right <= 445), isTrue);
    expect(rowsOf(left), 2);
    final perRow = left.where((r) => r.top == left.first.top).length;
    expect(perRow, inInclusiveRange(2, 4));
    expect(rowsOf(tiles(tester, find.byType(DeckPage).last)), 1, reason: '3 buttons fit in one row');
    expect(find.text('Последняя'), findsNothing);
    for (var i = 0; i < 6 && find.text('Последняя').evaluate().isEmpty; i++) {
      await tester.drag(find.descendant(of: find.byType(DeckPage).first, matching: find.byType(PageView)), const Offset(-300, 0));
      await tester.pumpAndSettle();
    }
    expect(find.text('Последняя'), findsOneWidget);
    expect(find.text('Jira'), findsOneWidget, reason: 'the right deck did not move');
  });

  testWidgets('one computer, or portrait: the whole screen is one deck', (tester) async {
    await show(tester, _App(home, null), const Size(890, 410));
    expect(find.byType(DeckPage), findsOneWidget);
    expect(find.text('Рабочий'), findsNothing);
    // Many buttons (page dots) still leave 2 rows, about twice as many per row as a half.
    final full = tiles(tester, find.byType(DeckPage));
    expect(rowsOf(full), 2);
    expect(full.where((r) => r.top == full.first.top).length, inInclusiveRange(5, 8));

    await show(tester, _App(home, work), const Size(410, 890));
    expect(find.byType(DeckPage), findsOneWidget);
    expect(find.text('Домашний'), findsOneWidget);
    expect(find.text('Jira'), findsNothing);
  });

  testWidgets('a computer that stays away is dimmed under a banner; the grid does not move', (tester) async {
    final pc = machine('x', 'Ноутбук', ['Почта', 'Музыка', 'Фото']);
    await show(tester, _App(pc, null), const Size(890, 410));
    final before = tiles(tester, find.byType(DeckPage));
    expect(find.text('Нет связи — показана последняя дека'), findsNothing);

    pc.status = LinkStatus.offline; // never came online in this session: nothing to wait for
    await show(tester, _App(pc, null), const Size(890, 410));
    expect(find.text('Нет связи — показана последняя дека'), findsOneWidget);
    expect(find.text('не в сети'), findsOneWidget);
    expect(tiles(tester, find.byType(DeckPage)), before);
    expect(tester.widget<DeckTile>(find.byType(DeckTile).first).enabled, isFalse);
  });
}
