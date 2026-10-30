import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/l10n/l10n.dart';
import 'package:fl_clash/manager/status_manager.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/pages/editor.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:fl_clash/state.dart';
import 'package:fl_clash/views/config/scripts.dart';
import 'package:fl_clash/views/profiles/overwrite/script.dart';
import 'package:fl_clash/widgets/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import '../helpers/test_app.dart';
import '../helpers/test_database_providers.dart';
import '../helpers/test_profiles.dart';

class _SaveFilePicker extends FilePickerPlatform {
  final called = Completer<void>();
  int calls = 0;
  String? name;
  Uint8List? savedBytes;
  Uri? result;
  Exception? error;
  Completer<Uri?>? pending;

  @override
  Future<Uri?> saveFile({
    required String fileName,
    required Uint8List bytes,
    required String mimeType,
    String? dialogTitle,
    String? initialDirectory,
    Function(FilePickerStatus)? onFileSaving,
    WindowsOptions windowsOptions = const WindowsOptions(),
    LinuxOptions linuxOptions = const LinuxOptions(),
    WebOptions webOptions = const WebOptions(),
  }) async {
    calls++;
    name = fileName;
    savedBytes = Uint8List.fromList(bytes);
    if (!called.isCompleted) called.complete();
    if (error != null) throw error!;
    return pending == null ? result : await pending!.future;
  }
}

Script _script(String label) {
  return Script(id: 10, label: label, lastUpdateTime: DateTime(2026));
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('scriptExportFileName', () {
    final cases = <String, String>{
      'alpha': 'alpha.ts',
      'alpha.js': 'alpha.ts',
      'alpha.JS': 'alpha.ts',
      'alpha.Ts': 'alpha.ts',
      'alpha.ts.ts': 'alpha.ts',
      'alpha.js.TS.Js': 'alpha.ts',
      'alpha.ts .JS ..ts': 'alpha.ts',
      'alpha.js... ': 'alpha.ts',
      'alpha .js': 'alpha.ts',
      '  \u8986\u5199\u811a\u672c.TS  ': '\u8986\u5199\u811a\u672c.ts',
      'a<>:"/\\|?*b.js': 'a_________b.ts',
      'a\x00\x1f\x7f\x85b.ts': 'a____b.ts',
      'alpha... ': 'alpha.ts',
      'alpha.txt': 'alpha.txt.ts',
      '': 'script_10.ts',
      '   ': 'script_10.ts',
      '...': 'script_10.ts',
      '.ts': 'script_10.ts',
      'CON': 'script_10.ts',
      'con.js': 'script_10.ts',
      'con .txt.ts': 'script_10.ts',
      'NUL.txt.ts': 'script_10.ts',
      'PrN': 'script_10.ts',
      'aux.TS': 'script_10.ts',
      'COM1': 'script_10.ts',
      'lpt9.js': 'script_10.ts',
      'COM\u00b9.js': 'script_10.ts',
      'CONIN\$': 'script_10.ts',
      'com0.js': 'com0.ts',
      'company.js': 'company.ts',
      'lpt10.ts': 'lpt10.ts',
    };
    for (final entry in cases.entries) {
      test('normalizes ${entry.key}', () {
        expect(scriptExportFileName(_script(entry.key)), entry.value);
      });
    }
    test('uses the actual id for fallback', () {
      expect(
        scriptExportFileName(_script('').copyWith(id: 42)),
        'script_42.ts',
      );
    });
  });

  late Directory home;
  late File source;
  late _SaveFilePicker filePicker;
  late FilePickerPlatform previousPicker;
  late ProviderContainer container;
  final supportDirectory = AppPath.supportDirectory;
  final temporaryDirectory = AppPath.temporaryDirectory;
  final cacheDirectory = AppPath.cacheDirectory;
  final downloadDirectory = AppPath.downloadDirectory;
  final script = _script('actual.JS');
  final storedBytes = Uint8List.fromList([
    0xef,
    0xbb,
    0xbf,
    ...'function main(config) { return config; }'.codeUnits,
    13,
    10,
    0xff,
  ]);

  setUpAll(() async {
    home = Directory.systemTemp.createTempSync('flclash-script-export-');
    AppPath.supportDirectory = () async => home;
    AppPath.temporaryDirectory = () async => home;
    AppPath.cacheDirectory = () async => home;
    AppPath.downloadDirectory = () async => home;
    await AppLocalizations.load(const Locale('en'));
    source = File(await script.path);
    await source.parent.create(recursive: true);
  });

  tearDownAll(() {
    AppPath.supportDirectory = supportDirectory;
    AppPath.temporaryDirectory = temporaryDirectory;
    AppPath.cacheDirectory = cacheDirectory;
    AppPath.downloadDirectory = downloadDirectory;
    home.deleteSync(recursive: true);
  });

  setUp(() {
    source.writeAsBytesSync(storedBytes);
    previousPicker = FilePickerPlatform.instance;
    filePicker = _SaveFilePicker();
    FilePickerPlatform.instance = filePicker;
    final profile = Profile.normal(
      label: 'p',
    ).copyWith(id: 1, overwriteType: OverwriteType.script, scriptId: 10);
    container = ProviderContainer(
      overrides: [
        scriptsProvider.overrideWith(() => TestScripts([script])),
        profilesProvider.overrideWith(() => TestProfiles([profile])),
        currentProfileIdProvider.overrideWithBuild((_, _) => 1),
        viewSizeProvider.overrideWithBuild((_, _) => const Size(800, 600)),
        isMobileViewProvider.overrideWithValue(false),
        overlayTopOffsetProvider.overrideWithValue(0),
      ],
    );
    globalState.container = container;
  });

  tearDown(() {
    FilePickerPlatform.instance = previousPicker;
    container.dispose();
  });

  Future<void> pump(WidgetTester tester, Widget child) async {
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MediaQuery(
          data: const MediaQueryData(size: Size(800, 600)),
          child: Directionality(
            textDirection: TextDirection.ltr,
            child: StatusManager(
              child: TestApp(child: Scaffold(body: child)),
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  Finder exportButton() => find.widgetWithIcon(IconButton, Icons.save_alt);

  Future<void> exportAndWait(WidgetTester tester) async {
    final callback = tester.widget<IconButton>(exportButton()).onPressed!;
    await tester.runAsync(callback as Future<void> Function());
    await tester.pump();
  }

  Future<void> tapAndWaitForPicker(WidgetTester tester) async {
    await tester.runAsync(() async {
      await tester.tap(exportButton());
      await filePicker.called.future;
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();
  }

  testWidgets('exports stored bytes and reports file URI success', (
    tester,
  ) async {
    final destination = File('${home.path}/export.ts');
    filePicker.result = destination.uri;
    await pump(tester, ScriptExportButton(script: script));

    await exportAndWait(tester);

    expect(find.byTooltip(currentAppLocalizations.exportFile), findsOneWidget);
    expect(filePicker.name, 'actual.ts');
    expect(filePicker.savedBytes, storedBytes);
    expect(destination.readAsBytesSync(), storedBytes);
    expect(source.readAsBytesSync(), storedBytes);
    expect(source.path, endsWith('10.js'));
    expect(find.text(currentAppLocalizations.exportSuccess), findsOneWidget);
    expect(find.byIcon(MessageLevel.success.icon!), findsOneWidget);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('accepts an Android content URI as success', (tester) async {
    filePicker.result = Uri.parse('content://documents/export/actual.ts');
    await pump(tester, ScriptExportButton(script: script));

    await exportAndWait(tester);

    expect(filePicker.savedBytes, storedBytes);
    expect(find.text(currentAppLocalizations.exportSuccess), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('cancellation is silent and re-enables export', (tester) async {
    await pump(tester, ScriptExportButton(script: script));

    await exportAndWait(tester);

    expect(filePicker.calls, 1);
    expect(find.byIcon(MessageLevel.success.icon!), findsNothing);
    expect(find.byIcon(MessageLevel.error.icon!), findsNothing);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('save failure is reported and can be retried', (tester) async {
    filePicker.error = Exception('save failed');
    await pump(tester, ScriptExportButton(script: script));

    await exportAndWait(tester);

    expect(find.text('Exception: save failed'), findsOneWidget);
    expect(find.byIcon(MessageLevel.error.icon!), findsOneWidget);
    expect(find.text(currentAppLocalizations.exportSuccess), findsNothing);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    filePicker.error = null;
    filePicker.result = Uri.parse('content://documents/export/actual.ts');
    await exportAndWait(tester);
    expect(filePicker.calls, 2);
    expect(find.text(currentAppLocalizations.exportSuccess), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('missing source reports an error without exporting a template', (
    tester,
  ) async {
    source.deleteSync();
    await pump(tester, ScriptExportButton(script: script));

    await exportAndWait(tester);

    expect(filePicker.calls, 0);
    expect(find.textContaining('FileSystemException'), findsOneWidget);
    expect(find.byIcon(MessageLevel.error.icon!), findsOneWidget);
    expect(find.text(currentAppLocalizations.exportSuccess), findsNothing);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    expect(source.existsSync(), isFalse);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('rapid and pending taps start only one export', (tester) async {
    filePicker.pending = Completer<Uri?>();
    await pump(tester, ScriptExportButton(script: script));

    await tester.runAsync(() async {
      await tester.tap(exportButton());
      await tester.tap(exportButton());
      await filePicker.called.future;
    });
    await tester.pump();
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    await tester.tap(exportButton());
    expect(filePicker.calls, 1);

    await tester.runAsync(() async {
      filePicker.pending!.complete(null);
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('completion after disposal neither notifies nor sets state', (
    tester,
  ) async {
    filePicker.pending = Completer<Uri?>();
    await pump(tester, ScriptExportButton(script: script));
    await tapAndWaitForPicker(tester);
    await pump(tester, const SizedBox.shrink());

    await tester.runAsync(() async {
      filePicker.pending!.complete(
        Uri.parse('content://documents/export/actual.ts'),
      );
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();

    expect(find.text(currentAppLocalizations.exportSuccess), findsNothing);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('configuration export does not select or open the editor', (
    tester,
  ) async {
    filePicker.pending = Completer<Uri?>();
    await pump(tester, const ScriptsView());

    await tapAndWaitForPicker(tester);
    await tester.tap(exportButton());
    await tester.pump();

    expect(filePicker.calls, 1);
    expect(filePicker.savedBytes, storedBytes);
    expect(
      tester
          .widget<SelectedDecorationListItem>(
            find.byType(SelectedDecorationListItem),
          )
          .isSelected,
      isFalse,
    );
    expect(find.text(currentAppLocalizations.selectAll), findsNothing);
    expect(find.byType(EditorPage), findsNothing);
    await tester.runAsync(() async {
      filePicker.pending!.complete(null);
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();

    await tester.tap(find.byType(CommonCheckBox));
    await tester.pumpAndSettle();
    expect(find.byType(ScriptExportButton), findsNothing);
    expect(find.text(currentAppLocalizations.selectAll), findsOneWidget);
    await tester.tap(find.byType(CommonCheckBox));
    await tester.pumpAndSettle();
    expect(find.byType(ScriptExportButton), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('profile export leaves the selected script unchanged', (
    tester,
  ) async {
    filePicker.pending = Completer<Uri?>();
    await pump(
      tester,
      const ProfileIdProvider(
        profileId: 1,
        child: CustomScrollView(slivers: [ScriptContent()]),
      ),
    );

    await tapAndWaitForPicker(tester);
    await tester.tap(exportButton());
    await tester.pump();

    expect(filePicker.calls, 1);
    expect(filePicker.savedBytes, storedBytes);
    expect(container.read(profilesProvider).getProfile(1)?.scriptId, 10);
    expect(
      tester.widget<RadioGroup<int>>(find.byType(RadioGroup<int>)).groupValue,
      10,
    );
    expect(find.byType(EditorPage), findsNothing);
    await tester.runAsync(() async {
      filePicker.pending!.complete(null);
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();
    await tester.pumpWidget(const SizedBox.shrink());
  });
}
