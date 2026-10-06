import 'package:barphone/protocol/link.dart';
import 'package:barphone/protocol/models.dart';
import 'package:barphone/state.dart';
import 'package:barphone/ui/deck_page.dart';
import 'package:barphone/ui/theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

Map<String, dynamic> stateMsg({required List<Map<String, dynamic>> profiles, required String active}) => {
  'type': 'state',
  'machine': {'id': 'm', 'name': 'Рабочий ПК', 'os': 'windows'},
  'deck': profiles.firstWhere((p) => p['id'] == active),
  'profiles': profiles,
  'activeProfile': active,
  'recent': <dynamic>[],
};

const defaultProfile = {
  'id': 'default',
  'name': 'Основной',
  'columns': 3,
  'buttons': [
    {'id': 'a', 'title': 'Почта', 'kind': 'url'},
    {'id': 'b', 'title': 'Музыка', 'kind': 'system', 'glyph': 'media_play_pause'},
  ],
};
const vsProfile = {
  'id': 'vs',
  'name': 'VS Code',
  'columns': 3,
  'buttons': [
    {'id': 'p', 'title': 'Палитра', 'kind': 'keys', 'glyph': 'keys'},
  ],
};

/// Records pins instead of persisting them.
class _App extends AppState {
  _App(this.link);
  final MachineLink link;
  final pins = <String?>[];

  @override
  void setPinnedProfile(String id, String? profileId) {
    pins.add(profileId);
    link.update(link.machine.copyWith(pinnedProfile: (profileId,)));
    notifyListeners();
  }
}

void main() {
  testWidgets('deck follows the PC profile; the chip pins another; a deleted pin falls back', (tester) async {
    final link = MachineLink(
      SavedMachine(
        id: 'm',
        name: 'Рабочий ПК',
        os: 'windows',
        port: 1,
        hosts: const ['127.0.0.1'],
        token: 't',
        lastState: stateMsg(profiles: [defaultProfile, vsProfile], active: 'vs'),
      ),
      onChanged: () {},
      onMachineUpdated: (_) {},
    );
    final app = _App(link);
    await tester.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        home: Scaffold(
          body: ListenableBuilder(
            listenable: app,
            builder: (context, _) => DeckPage(app: app, link: link, onShowMachines: () {}, onAddMachine: () {}),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    // The PC has VS Code in front: its profile is shown, the chip says so (auto).
    expect(find.text('Палитра'), findsOneWidget);
    expect(find.text('Почта'), findsNothing);
    expect(find.text('VS Code'), findsOneWidget);
    expect(find.byIcon(Icons.auto_awesome_rounded), findsOneWidget);

    // Pin the default profile from the picker.
    await tester.tap(find.text('VS Code'));
    await tester.pumpAndSettle();
    expect(find.text('Профиль деки'), findsOneWidget);
    expect(find.text('По приложению на компьютере · сейчас «VS Code»'), findsOneWidget);
    await tester.tap(find.text('Основной'));
    await tester.pumpAndSettle();
    expect(app.pins, ['default']);
    expect(find.text('Почта'), findsOneWidget);
    expect(find.text('Палитра'), findsNothing);
    expect(find.byIcon(Icons.push_pin_rounded), findsOneWidget);

    // The pinned profile is deleted on the PC: the phone follows the PC again.
    app.setPinnedProfile('m', 'vs');
    await tester.pumpAndSettle();
    link.state = DeckState.fromJson(stateMsg(profiles: [defaultProfile], active: 'default'));
    app.notifyListeners();
    await tester.pumpAndSettle();
    expect(app.pins.last, isNull);
    expect(find.text('Почта'), findsOneWidget);
    expect(find.byIcon(Icons.push_pin_rounded), findsNothing);
    expect(find.text('Основной'), findsNothing, reason: 'with a single profile there is no chip');
  });
}
