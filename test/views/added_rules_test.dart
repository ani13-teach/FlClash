import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:fl_clash/common/added_rules.dart';
import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/manager/status_manager.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/providers/providers.dart';
import 'package:fl_clash/state.dart';
import 'package:fl_clash/views/config/rules.dart';
import 'package:fl_clash/widgets/widgets.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../helpers/test_app.dart';
import '../helpers/test_database_providers.dart';

class _RulesFilePicker extends FilePickerPlatform {
  int calls = 0;
  Uint8List? savedBytes;
  String? name;
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
    savedBytes = Uint8List.fromList(bytes);
    name = fileName;
    if (error != null) throw error!;
    return pending == null ? result : await pending!.future;
  }
}

class _RecordingGlobalRules extends TestGlobalRules {
  _RecordingGlobalRules(super.initial);

  final deleted = <List<int>>[];

  @override
  void delAll(Iterable<int> ruleIds) {
    deleted.add(List<int>.from(ruleIds));
  }
}

Rule _rule(int id, String content) {
  return Rule(
    id: id,
    content: content,
    ruleTarget: 'DIRECT',
    order: id.toString(),
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late ProviderContainer container;
  late _RecordingGlobalRules rules;
  late _RulesFilePicker filePicker;
  late FilePickerPlatform previousPicker;
  late Directory home;
  final downloadDirectory = AppPath.downloadDirectory;

  setUpAll(() {
    home = Directory.systemTemp.createTempSync('flclash-rules-transfer-');
    AppPath.downloadDirectory = () async => home;
  });

  tearDownAll(() {
    AppPath.downloadDirectory = downloadDirectory;
    home.deleteSync(recursive: true);
  });

  setUp(() {
    previousPicker = FilePickerPlatform.instance;
    filePicker = _RulesFilePicker();
    FilePickerPlatform.instance = filePicker;
  });

  tearDown(() {
    FilePickerPlatform.instance = previousPicker;
  });

  Future<void> pumpRules(
    WidgetTester tester,
    List<Rule> initial, {
    Size size = const Size(1200, 2400),
  }) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    rules = _RecordingGlobalRules(initial);
    container = ProviderContainer(
      overrides: [globalRulesProvider.overrideWith(() => rules)],
    );
    addTearDown(container.dispose);
    globalState.container = container;
    container.read(viewSizeProvider.notifier).update((_) => size);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MediaQuery(
          data: MediaQueryData(size: size),
          child: const Directionality(
            textDirection: TextDirection.ltr,
            child: StatusManager(child: TestApp(child: AddedRulesView())),
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

  testWidgets('shows file actions and disables export for an empty list', (
    tester,
  ) async {
    await pumpRules(tester, const []);

    expect(find.byTooltip(currentAppLocalizations.importFile), findsOneWidget);
    expect(find.byTooltip(currentAppLocalizations.exportFile), findsOneWidget);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNull);
  });

  testWidgets('exports all rules in display order to a YAML file', (
    tester,
  ) async {
    final destination = File('${home.path}/additional_rules.yaml');
    filePicker.result = destination.uri;
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    await exportAndWait(tester);

    expect(filePicker.name, 'additional_rules.yaml');
    expect(destination.readAsBytesSync(), filePicker.savedBytes);
    expect(
      decodeAddedRules(
        utf8.decode(filePicker.savedBytes!),
      ).map((rule) => rule.content),
      ['a.com', 'b.com'],
    );
    expect(find.text(currentAppLocalizations.exportSuccess), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('exports only checked rules and keeps selection', (tester) async {
    filePicker.result = Uri.parse('content://documents/rules.yaml');
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);
    await tester.tap(find.byType(CommonCheckBox).last);
    await tester.pumpAndSettle();

    await exportAndWait(tester);

    expect(
      decodeAddedRules(utf8.decode(filePicker.savedBytes!)).single.content,
      'b.com',
    );
    expect(find.text(currentAppLocalizations.selectAll), findsOneWidget);
    expect(find.byTooltip(currentAppLocalizations.importFile), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('export cancellation is silent and permits retry', (
    tester,
  ) async {
    await pumpRules(tester, [_rule(1, 'a.com')]);

    await exportAndWait(tester);

    expect(filePicker.calls, 1);
    expect(find.text(currentAppLocalizations.exportSuccess), findsNothing);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
  });

  testWidgets('failed export reports an error and permits retry', (
    tester,
  ) async {
    filePicker.error = Exception('save failed');
    await pumpRules(tester, [_rule(1, 'a.com')]);

    await exportAndWait(tester);

    expect(find.text('Exception: save failed'), findsOneWidget);
    expect(find.text(currentAppLocalizations.exportSuccess), findsNothing);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNotNull);
    filePicker.error = null;
    await exportAndWait(tester);
    expect(filePicker.calls, 2);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('pending exports ignore repeated calls and survive disposal', (
    tester,
  ) async {
    filePicker.pending = Completer<Uri?>();
    await pumpRules(tester, [_rule(1, 'a.com')]);
    final callback =
        tester.widget<IconButton>(exportButton()).onPressed!
            as Future<void> Function();
    late Future<void> pending;
    await tester.runAsync(() async {
      pending = callback();
      await callback();
      await Future<void>.delayed(Duration.zero);
    });
    await tester.pump();

    expect(filePicker.calls, 1);
    expect(tester.widget<IconButton>(exportButton()).onPressed, isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.runAsync(() async {
      filePicker.pending!.complete(Uri.parse('content://documents/rules.yaml'));
      await pending;
    });
    await tester.pump();
    expect(tester.takeException(), isNull);
  });

  testWidgets('file actions fit a narrow window in both selection states', (
    tester,
  ) async {
    await pumpRules(tester, [_rule(1, 'a.com')], size: const Size(390, 844));

    expect(tester.takeException(), isNull);
    await tester.tap(find.byType(CommonCheckBox).first);
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(exportButton().hitTestable(), findsOneWidget);
  });

  testWidgets('shows the empty state without any rule', (tester) async {
    await pumpRules(tester, const []);

    expect(find.byType(NullStatus), findsOneWidget);
    expect(find.text(currentAppLocalizations.add), findsOneWidget);
  });

  testWidgets('lists every rule when the store is populated', (tester) async {
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    expect(find.byType(NullStatus), findsNothing);
    expect(find.text('a.com'), findsOneWidget);
    expect(find.text('b.com'), findsOneWidget);
  });

  testWidgets('checking a rule enters selection mode', (tester) async {
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    expect(find.text(currentAppLocalizations.selectAll), findsNothing);

    await tester.tap(find.byType(CommonCheckBox).first);
    await tester.pumpAndSettle();

    expect(find.text(currentAppLocalizations.selectAll), findsOneWidget);
    expect(find.byIcon(Icons.delete), findsOneWidget);
  });

  testWidgets('select all covers every rule and toggles back off', (
    tester,
  ) async {
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    await tester.tap(find.byType(CommonCheckBox).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text(currentAppLocalizations.selectAll));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.delete));
    await tester.pumpAndSettle();
    await tester.tap(find.text(currentAppLocalizations.confirm));
    await tester.pumpAndSettle();

    expect(rules.deleted, [
      unorderedEquals(<int>[1, 2]),
    ]);
  });

  testWidgets('declining the delete dialog keeps every rule', (tester) async {
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    await tester.tap(find.byType(CommonCheckBox).first);
    await tester.pumpAndSettle();
    await tester.tap(find.byIcon(Icons.delete));
    await tester.pumpAndSettle();
    await tester.tap(find.text(currentAppLocalizations.cancel));
    await tester.pumpAndSettle();

    expect(rules.deleted, isEmpty);
    expect(find.text(currentAppLocalizations.selectAll), findsOneWidget);
  });

  testWidgets('deleting only removes the selected rule', (tester) async {
    await pumpRules(tester, [_rule(1, 'a.com'), _rule(2, 'b.com')]);

    await tester.tap(find.byType(CommonCheckBox).last);
    await tester.pumpAndSettle();
    await tester.tap(find.byIcon(Icons.delete));
    await tester.pumpAndSettle();
    await tester.tap(find.text(currentAppLocalizations.confirm));
    await tester.pumpAndSettle();

    expect(rules.deleted, [
      [2],
    ]);
  });
}
