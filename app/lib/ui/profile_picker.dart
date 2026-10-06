import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../state.dart';
import 'theme.dart';

/// Bottom sheet: follow the PC (the profile switches with the app in front) or pin one
/// profile on this phone.
Future<void> showProfilePicker(BuildContext context, AppState app, MachineLink link) async {
  final st = link.state;
  if (st == null) return;
  final pinned = link.machine.pinnedProfile;
  final pcChoice = st.profile(st.activeProfile) ?? st.profiles.first;
  HapticFeedback.selectionClick();
  await showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(context).height * .8),
    builder: (sheet) {
      void pick(String? id) {
        app.setPinnedProfile(link.machine.id, id);
        Navigator.pop(sheet);
      }

      Widget option({
        required IconData icon,
        required String title,
        String? subtitle,
        required bool selected,
        required VoidCallback onTap,
      }) => ListTile(
        leading: Icon(icon, color: selected ? C.accent : C.muted),
        title: Text(title, style: TextStyle(fontWeight: selected ? FontWeight.w700 : FontWeight.w500)),
        subtitle: subtitle == null ? null : Text(subtitle, style: const TextStyle(color: C.muted, fontSize: 12.5)),
        trailing: selected ? const Icon(Icons.check_rounded, color: C.accent) : null,
        onTap: onTap,
      );

      return SafeArea(
        child: ListView(
          shrinkWrap: true,
          children: [
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 0, 20, 8),
              child: Text('Профиль деки', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
            ),
            option(
              icon: Icons.auto_awesome_rounded,
              title: 'Автоматически',
              subtitle: 'По приложению на компьютере · сейчас «${pcChoice.name}»',
              selected: pinned == null,
              onTap: () => pick(null),
            ),
            const Divider(height: 8, indent: 20, endIndent: 20),
            for (final p in st.profiles)
              option(
                icon: Icons.push_pin_outlined,
                title: p.name,
                subtitle: 'Закрепить · ${p.buttons.length} ${_buttonsWord(p.buttons.length)}',
                selected: pinned == p.id,
                onTap: () => pick(p.id),
              ),
            const SizedBox(height: 8),
          ],
        ),
      );
    },
  );
}

String _buttonsWord(int n) {
  final m10 = n % 10, m100 = n % 100;
  if (m10 == 1 && m100 != 11) return 'кнопка';
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 'кнопки';
  return 'кнопок';
}
