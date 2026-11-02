import 'package:fl_clash/common/constant.dart';
import 'package:fl_clash/l10n/l10n.dart';
import 'package:fl_clash/providers/app.dart';
import 'package:fl_clash/providers/config.dart';
import 'package:fl_clash/providers/database.dart';
import 'package:fl_clash/state.dart';
import 'package:fl_clash/views/config/general.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../helpers/test_app.dart';
import '../helpers/test_profiles.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer(
      overrides: [profilesProvider.overrideWith(TestProfiles.new)],
    );
    globalState.container = container;
    container.read(viewSizeProvider.notifier).value = const Size(1200, 1000);
  });

  tearDown(() => container.dispose());

  Future<void> pumpItem(WidgetTester tester, Widget item) async {
    tester.view.physicalSize = const Size(1200, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: TestApp(
          locale: const Locale('en'),
          child: Scaffold(body: ListView(children: [item])),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  Future<void> openInput(WidgetTester tester) async {
    await tester.tap(find.byType(ListTile).first);
    await tester.pumpAndSettle();
  }

  Future<void> submit(WidgetTester tester, String value) async {
    await tester.enterText(find.byType(TextFormField), value);
    await tester.tap(find.text(AppLocalizations.current.submit));
    await tester.pumpAndSettle();
  }

  testWidgets('timeout validates, saves, cancels and resets', (tester) async {
    await pumpItem(tester, const StickyTimeoutItem());
    expect(container.read(appSettingProvider).stickyTimeout, 5);
    await openInput(tester);
    for (final invalid in ['', '0', '-1', '61', '1.5', 'abc']) {
      await submit(tester, invalid);
      expect(
        find.text(AppLocalizations.current.stickyTimeoutRange),
        findsOneWidget,
      );
      expect(container.read(appSettingProvider).stickyTimeout, 5);
    }
    await submit(tester, '12');
    expect(container.read(appSettingProvider).stickyTimeout, 12);
    await openInput(tester);
    await tester.enterText(find.byType(TextFormField), '20');
    Navigator.of(tester.element(find.byType(TextFormField))).pop();
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).stickyTimeout, 12);
    await openInput(tester);
    await tester.tap(find.text(AppLocalizations.current.reset));
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).stickyTimeout, 5);
  });

  testWidgets('failure threshold accepts every integer from 1 to 10', (
    tester,
  ) async {
    await pumpItem(tester, const StickyFailureThresholdItem());
    expect(container.read(appSettingProvider).stickyFailureThreshold, 2);
    await openInput(tester);
    for (final invalid in ['', '0', '-1', '11', '1.5', 'abc']) {
      await submit(tester, invalid);
      expect(
        find.text(AppLocalizations.current.stickyFailureThresholdRange),
        findsOneWidget,
      );
      expect(container.read(appSettingProvider).stickyFailureThreshold, 2);
    }
    for (var count = 1; count <= 10; count++) {
      await submit(tester, '$count');
      expect(container.read(appSettingProvider).stickyFailureThreshold, count);
      await openInput(tester);
    }
    await tester.tap(find.text(AppLocalizations.current.reset));
    await tester.pumpAndSettle();
    expect(container.read(appSettingProvider).stickyFailureThreshold, 2);
  });

  testWidgets(
    'probe URL validates, saves and resets without changing manual URL',
    (tester) async {
      await pumpItem(tester, const StickyTestUrlItem());
      expect(find.text(defaultStickyTestUrl), findsOneWidget);
      await openInput(tester);
      for (final invalid in [
        '',
        'ftp://example.com',
        'https://',
        'https://u:p@example.com',
      ]) {
        await submit(tester, invalid);
        expect(find.byType(TextFormField), findsOneWidget);
        expect(
          container.read(appSettingProvider).stickyTestUrl,
          defaultStickyTestUrl,
        );
      }
      await submit(tester, ' https://health.example/204 ');
      expect(
        container.read(appSettingProvider).stickyTestUrl,
        'https://health.example/204',
      );
      expect(container.read(appSettingProvider).testUrl, defaultTestUrl);
      await openInput(tester);
      await tester.tap(find.text(AppLocalizations.current.reset));
      await tester.pumpAndSettle();
      expect(
        container.read(appSettingProvider).stickyTestUrl,
        defaultStickyTestUrl,
      );
    },
  );
}
