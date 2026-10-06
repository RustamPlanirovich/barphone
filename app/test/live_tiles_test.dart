import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/ui/theme.dart';
import 'package:barphone/ui/tile.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('a live tile shows its glyph until numbers come, then the value', (tester) async {
    final link = MachineLink(
      const SavedMachine(id: 'm', name: 'ПК', os: 'windows', port: 1, hosts: ['127.0.0.1'], token: 't'),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    Future<void> pump(DeckButton b) => tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: Center(
            child: DeckTile(button: b, link: link, size: 140, enabled: true),
          ),
        ),
      ),
    );
    const cpu = DeckButton(id: 'c', title: 'Процессор', kind: 'stat', glyph: 'cpu', stat: 'cpu');
    const ram = DeckButton(id: 'r', title: 'Память', kind: 'stat', glyph: 'ram', stat: 'ram');
    await pump(cpu);
    expect(find.byIcon(Icons.speed_rounded), findsOneWidget);

    link.stats.value = SysStats.fromJson({'cpu': 0.234, 'ram': 0.61, 'ramUsed': 10.5 * (1 << 30), 'ramTotal': 32 << 30});
    await tester.pump();
    expect(find.text('23%'), findsOneWidget);
    expect(find.byType(LinearProgressIndicator), findsOneWidget);

    await pump(ram);
    expect(find.text('61%'), findsOneWidget);
    expect(find.text('11 / 32 ГБ'), findsOneWidget);

    link.stats.value = SysStats.fromJson({'cpu': 0.5}); // memory not measured this time
    await tester.pump();
    expect(find.byIcon(Icons.memory_rounded), findsOneWidget);
  });
}
