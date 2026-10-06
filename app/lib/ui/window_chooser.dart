import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';
import 'tile.dart';

/// What the user picked in the chooser: a window, or (null) a new instance.
class _Pick {
  final AppWindow? window;
  const _Pick(this.window);
}

/// Bottom sheet listing the app's open windows on the PC plus "start a new one".
/// Shown when a press finds several windows, and on long press.
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
    constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(context).height * .8),
    builder: (sheet) => SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                ButtonIcon(button: b, link: link, size: 44),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        b.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        windows.isEmpty ? 'Открытых окон нет' : 'Уже открыто окон: ${windows.length}. Куда переключиться?',
                        style: const TextStyle(color: C.muted, fontSize: 13),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 14),
            Flexible(
              child: ListView.separated(
                shrinkWrap: true,
                itemCount: windows.length,
                separatorBuilder: (_, _) => const SizedBox(height: 8),
                itemBuilder: (_, i) => Material(
                  color: C.tile,
                  borderRadius: BorderRadius.circular(14),
                  child: InkWell(
                    borderRadius: BorderRadius.circular(14),
                    onTap: () => Navigator.pop(sheet, _Pick(windows[i])),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                      child: Row(
                        children: [
                          const Icon(Icons.web_asset_rounded, color: C.muted, size: 22),
                          const SizedBox(width: 12),
                          Expanded(
                            child: Text(
                              windows[i].title,
                              maxLines: 2,
                              overflow: TextOverflow.ellipsis,
                              style: const TextStyle(fontSize: 15),
                            ),
                          ),
                          const Icon(Icons.chevron_right_rounded, color: C.muted),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 14),
            FilledButton.icon(
              onPressed: () => Navigator.pop(sheet, const _Pick(null)),
              icon: const Icon(Icons.add_rounded),
              label: const Text('Запустить новый'),
            ),
          ],
        ),
      ),
    ),
  );
  if (pick == null) return;
  HapticFeedback.lightImpact();
  final res = pick.window == null ? await link.launch(b.id, newInstance: true) : await link.focus(b.id, pick.window!.id);
  flash(res.ok);
  if (!res.ok && context.mounted) showPressError(context, b, res.error);
}
