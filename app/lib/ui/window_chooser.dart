import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';
import 'tile.dart';

enum _Do { focus, minimize, minimizeAll, launchNew }

class _Pick {
  final _Do what;
  final AppWindow? window;
  const _Pick(this.what, [this.window]);
}

/// Bottom sheet with the app's open windows on the PC as tiles, plus "start a new one"
/// and "minimize all". Shown when a press finds several windows, and on long press.
/// Tapping the window that is already in front minimizes it, like the deck button does.
Future<void> showWindowChooser(
  BuildContext context,
  MachineLink link,
  DeckButton b,
  List<AppWindow> windows, {
  required void Function(bool ok) flash,
}) async {
  final pick = await showModalBottomSheet<_Pick>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(context).height * .85, maxWidth: 900),
    builder: (sheet) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                ButtonIcon(button: b, link: link, size: 36),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        b.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                      ),
                      Text(
                        windows.isEmpty ? 'Открытых окон нет' : 'Открыто окон: ${windows.length}',
                        style: const TextStyle(color: C.muted, fontSize: 13),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Flexible(
              child: GridView(
                shrinkWrap: true,
                gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
                  maxCrossAxisExtent: 180,
                  mainAxisSpacing: 10,
                  crossAxisSpacing: 10,
                  childAspectRatio: 1.05,
                ),
                children: [
                  for (final w in windows)
                    _WindowTile(
                      button: b,
                      link: link,
                      window: w,
                      onTap: () => Navigator.pop(sheet, _Pick(w.active ? _Do.minimize : _Do.focus, w)),
                    ),
                  _ActionTile(
                    icon: Icons.add_rounded,
                    label: 'Запустить новый',
                    onTap: () => Navigator.pop(sheet, const _Pick(_Do.launchNew)),
                  ),
                  if (windows.isNotEmpty)
                    _ActionTile(
                      icon: Icons.minimize_rounded,
                      label: 'Свернуть все',
                      onTap: () => Navigator.pop(sheet, const _Pick(_Do.minimizeAll)),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    ),
  );
  if (pick == null) return;
  HapticFeedback.lightImpact();
  final res = switch (pick.what) {
    _Do.focus => await link.focus(b.id, pick.window!.id),
    _Do.minimize => await link.minimize(b.id, pick.window!.id),
    _Do.minimizeAll => await link.minimize(b.id),
    _Do.launchNew => await link.launch(b.id, newInstance: true),
  };
  flash(res.ok);
  if (!res.ok && context.mounted) showPressError(context, b, res.error);
}

class _WindowTile extends StatelessWidget {
  const _WindowTile({required this.button, required this.link, required this.window, required this.onTap});
  final DeckButton button;
  final MachineLink link;
  final AppWindow window;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final active = window.active;
    return Material(
      color: active ? C.tilePressed : C.tile,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(18),
        side: BorderSide(color: active ? C.accent : Colors.transparent, width: 1.5),
      ),
      child: InkWell(
        customBorder: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  ButtonIcon(button: button, link: link, size: 30),
                  if (active)
                    Flexible(
                      child: Container(
                        margin: const EdgeInsets.only(left: 8),
                        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                        decoration: BoxDecoration(color: C.accent.withValues(alpha: .2), borderRadius: BorderRadius.circular(8)),
                        child: const Text(
                          'впереди',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(fontSize: 11, color: C.accent, fontWeight: FontWeight.w600),
                        ),
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 10),
              Expanded(
                child: Text(
                  window.title,
                  maxLines: 3,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600, height: 1.2),
                ),
              ),
              Text(active ? 'тап — свернуть' : 'тап — перейти', style: const TextStyle(fontSize: 11, color: C.muted)),
            ],
          ),
        ),
      ),
    );
  }
}

class _ActionTile extends StatelessWidget {
  const _ActionTile({required this.icon, required this.label, required this.onTap});
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(18),
        side: const BorderSide(color: C.border, width: 1.5),
      ),
      child: InkWell(
        customBorder: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
        onTap: onTap,
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 32, color: C.accent),
            const SizedBox(height: 8),
            Text(
              label,
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
    );
  }
}
