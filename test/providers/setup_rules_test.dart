import 'dart:io';

import 'package:drift/native.dart';
import 'package:fl_clash/core/controller.dart';
import 'package:fl_clash/core/interface.dart';
import 'package:fl_clash/database/database.dart' as db;
import 'package:fl_clash/enum/enum.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/providers/action.dart';
import 'package:fl_clash/providers/core.dart';
import 'package:fl_clash/providers/database.dart';
import 'package:fl_clash/providers/state.dart';
import 'package:fl_clash/state.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:package_info_plus/package_info_plus.dart';
import 'package:path_provider_platform_interface/path_provider_platform_interface.dart';
import 'package:riverpod/riverpod.dart';
import 'package:yaml/yaml.dart';

import '../helpers/test_profiles.dart';

class _MockCore extends Mock implements CoreHandlerInterface {}

class _FakePathProvider extends PathProviderPlatform {
  _FakePathProvider(this.root);

  final String root;

  @override
  Future<String?> getApplicationSupportPath() async => root;

  @override
  Future<String?> getTemporaryPath() async => root;

  @override
  Future<String?> getApplicationCachePath() async => root;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const profile = Profile(
    id: 1,
    autoUpdateDuration: Duration.zero,
    overwriteType: OverwriteType.script,
  );
  const directRule = Rule(
    id: 1,
    content: '8888.name',
    ruleTarget: 'DIRECT',
    order: 'a',
  );
  const disabledRule = Rule(
    id: 2,
    content: 'disabled.example',
    ruleTarget: 'DIRECT',
    order: 'b',
  );
  const localRule = Rule(
    id: 3,
    content: 'standard-only.example',
    ruleTarget: 'DIRECT',
    order: 'a',
  );

  late db.Database database;
  late ProviderContainer container;
  late _MockCore core;
  late Directory tempDir;
  late PathProviderPlatform originalPathProvider;

  setUp(() async {
    database = db.Database(NativeDatabase.memory());
    db.database = database;
    await database.profilesDao.putAll([profile.toCompanion()]);
    await database.rulesDao.putGlobalRule(directRule);
    await database.rulesDao.putGlobalRule(disabledRule);
    await database.rulesDao.putDisabledLink(profile.id, disabledRule.id);
    await database.rulesDao.putProfileAddedRule(profile.id, localRule);
    core = _MockCore();
    container = ProviderContainer(
      overrides: [
        profilesProvider.overrideWith(() => TestProfiles([profile])),
        coreHandlerProvider.overrideWithValue(CoreController.scoped(core)),
      ],
    );
    tempDir = Directory.systemTemp.createTempSync('setup_rules_test');
    originalPathProvider = PathProviderPlatform.instance;
    PathProviderPlatform.instance = _FakePathProvider(tempDir.path);
  });

  setUpAll(() {
    globalState.packageInfo = PackageInfo(
      appName: 'FlClash',
      packageName: 'com.follow.clash',
      version: '0.0.0',
      buildNumber: '0',
    );
  });

  tearDown(() async {
    container.dispose();
    await database.close();
    PathProviderPlatform.instance = originalPathProvider;
    await tempDir.delete(recursive: true);
  });

  test(
    'script setup includes enabled global rules, not standard-only rules',
    () async {
      final state = await container.read(setupStateProvider(profile.id).future);

      expect(state.overwriteType, OverwriteType.script);
      expect(state.addedRules, [directRule]);
      expect(state.rules, isEmpty);
      expect(state.proxyGroups, isEmpty);
    },
  );

  test(
    'script mode prepends direct rules before subscription proxy rules',
    () async {
      when(() => core.getConfig(any())).thenAnswer(
        (_) async => {
          'rule': ['DOMAIN,8888.name,节点选择', 'MATCH,节点选择'],
        },
      );
      final state = await container.read(setupStateProvider(profile.id).future);
      final result = await container
          .read(setupActionProvider.notifier)
          .getProfile(setupState: state, patchConfig: const PatchClashConfig());

      expect((loadYaml(result.yaml) as YamlMap)['rules'], [
        'DOMAIN,8888.name,DIRECT',
        'DOMAIN,8888.name,节点选择',
        'MATCH,节点选择',
      ]);
    },
  );

  test(
    'standard setup still includes local rules before enabled globals',
    () async {
      container
          .read(profilesProvider.notifier)
          .put(profile.copyWith(overwriteType: OverwriteType.standard));
      final state = await container.read(setupStateProvider(profile.id).future);

      expect(state.addedRules, [localRule, directRule]);
    },
  );

  test('custom setup keeps its separate rule list', () async {
    container
        .read(profilesProvider.notifier)
        .put(profile.copyWith(overwriteType: OverwriteType.custom));
    await database.rulesDao.putProfileCustomRule(
      profile.id,
      const Rule(id: 4, ruleAction: RuleAction.MATCH, ruleTarget: 'DIRECT'),
    );
    final state = await container.read(setupStateProvider(profile.id).future);

    expect(state.addedRules, isEmpty);
    expect(state.rules.single.rawValue, 'MATCH,DIRECT');
  });
}
