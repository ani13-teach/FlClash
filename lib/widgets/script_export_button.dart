import 'dart:io';

import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/state.dart';
import 'package:material_ui/material_ui.dart';

String scriptExportFileName(Script script) {
  final trailing = RegExp(r'[. ]+$');
  final scriptExtension = RegExp(r'\.(js|ts)$', caseSensitive: false);
  var name = script.label
      .replaceAll(RegExp(r'[<>:"/\\|?*\x00-\x1f\x7f-\x9f]'), '_')
      .trim()
      .replaceFirst(trailing, '');
  while (scriptExtension.hasMatch(name)) {
    name = name.replaceFirst(scriptExtension, '').replaceFirst(trailing, '');
  }
  final reserved = RegExp(
    r'^(con|prn|aux|nul|conin\$|conout\$|com[1-9\u00b9\u00b2\u00b3]|lpt[1-9\u00b9\u00b2\u00b3]) *(?:\.|$)',
    caseSensitive: false,
  );
  if (name.isEmpty || reserved.hasMatch(name)) {
    name = 'script_${script.id}';
  }
  return '$name.ts';
}

class ScriptExportButton extends StatefulWidget {
  const ScriptExportButton({super.key, required this.script});

  final Script script;

  @override
  State<ScriptExportButton> createState() => _ScriptExportButtonState();
}

class _ScriptExportButtonState extends State<ScriptExportButton> {
  bool _exporting = false;

  Future<void> _export() async {
    if (_exporting) return;
    final script = widget.script;
    final appLocalizations = context.appLocalizations;
    setState(() => _exporting = true);
    try {
      final result = await globalState.safeRun<bool>(() async {
        final bytes = await File(await script.path).readAsBytes();
        final uri = await picker.saveFile(scriptExportFileName(script), bytes);
        return uri != null;
      }, title: appLocalizations.tip);
      if (result == true && mounted) {
        context.showNotifier(
          appLocalizations.exportSuccess,
          level: MessageLevel.success,
        );
      }
    } finally {
      if (mounted) {
        setState(() => _exporting = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return IconButton(
      tooltip: context.appLocalizations.exportFile,
      onPressed: _export,
      color: _exporting ? Theme.of(context).disabledColor : null,
      icon: const Icon(Icons.save_alt),
    );
  }
}
