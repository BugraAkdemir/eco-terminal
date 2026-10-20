import 'dart:convert';
import 'dart:io';
import 'package:dio/dio.dart';
import 'package:eco_terminal/core/api/providers.dart';
import 'package:eco_terminal/core/session/session_controller.dart';
import 'package:eco_terminal/features/observations/data/draft_queue.dart';
import 'package:eco_terminal/features/observations/data/draft_store_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'helpers/fakes.dart';

Future<ProviderContainer> queueContainer(
  MemoryDraftStore store,
  FakeAdapter adapter,
) async {
  SharedPreferences.setMockInitialValues({});
  final prefs = await SharedPreferences.getInstance();
  final c = ProviderContainer(
    overrides: [
      sharedPreferencesProvider.overrideWithValue(prefs),
      ...baseOverrides(adapter, token: 'tok', draftStore: store),
    ],
    retry: (_, _) => null,
  );
  c.listen(draftQueueProvider('u1'), (_, _) {});
  c.listen(draftQueueProvider('another'), (_, _) {});
  await c.read(sessionProvider.notifier).restore();
  return c;
}

FieldDraft sample(String id) => FieldDraft(
  id: id,
  body: {
    'category': 'animal',
    'title': 'Saha kaydı',
    'lat': 41.38,
    'lon': 33.78,
    'observed_at': '2026-10-19T10:00:00Z',
  },
);
Future<void> until(bool Function() ready) async {
  final end = DateTime.now().add(const Duration(seconds: 3));
  while (!ready()) {
    if (DateTime.now().isAfter(end)) fail('Queue did not reach expected state');
    await Future<void>.delayed(const Duration(milliseconds: 5));
  }
}

void main() {
  test(
    'lost create response retries with the same client_ref; persisted photos survive restart',
    () async {
      final store = MemoryDraftStore();
      final adapter = emptyApi()..on('GET /auth/me', {'data': userJson()});
      var creates = 0;
      final refs = <String>[];
      adapter.routes['POST /observations'] = (options) {
        creates++;
        refs.add((options.data as Map)['client_ref'] as String);
        if (creates == 1) {
          throw DioException(
            requestOptions: options,
            type: DioExceptionType.connectionError,
          );
        }
        return (
          200,
          {
            'data': {
              'id': 'server-one',
              'lat': 41.38,
              'lon': 33.78,
              'observed_at': '2026-10-19T10:00:00Z',
            },
          },
        );
      };
      adapter.on('POST /observations/server-one/media', {
        'data': {'id': 'media-one'},
      }, status: 201);
      var c = await queueContainer(store, adapter);
      await c.read(draftQueueProvider('u1').future);
      final draft = sample('client-ref-one').copy(
        photos: [
          DraftPhoto('field.png', base64Encode([1, 2, 3])),
        ],
      );
      await c.read(draftQueueProvider('u1').notifier).enqueue(draft);
      await until(
        () => c.read(draftQueueProvider('u1')).value?.first.attempts == 1,
      );
      expect(
        c.read(draftQueueProvider('u1')).requireValue.first.status,
        DraftStatus.queued,
      );
      c.dispose();
      expect(store.records['u1'], contains('field.png'));
      c = await queueContainer(store, adapter);
      addTearDown(c.dispose);
      await c.read(draftQueueProvider('u1').future);
      await c.read(draftQueueProvider('u1').notifier).sync(force: true);
      expect(refs, ['client-ref-one', 'client-ref-one']);
      final sent = c.read(draftQueueProvider('u1')).requireValue.first;
      expect(sent.status, DraftStatus.sent);
      expect(sent.serverId, 'server-one');
      expect(sent.photos, isEmpty);
      expect(
        adapter.requests.where((r) => r.path.endsWith('/media')).length,
        1,
      );
    },
  );
  test(
    'restart resumes remaining photo and never creates the observation again',
    () async {
      final store = MemoryDraftStore();
      final draft = sample('resume-client').copy(
        status: DraftStatus.sending,
        serverId: 'existing',
        uploaded: 1,
        photos: [
          DraftPhoto('one.png', base64Encode([1])),
          DraftPhoto('two.png', base64Encode([2])),
        ],
      );
      store.records['u1'] = jsonEncode([draft.toJson()]);
      final adapter = emptyApi()
        ..on('GET /auth/me', {'data': userJson()})
        ..on('POST /observations/existing/media', {
          'data': {'id': 'photo-two'},
        }, status: 201);
      final c = await queueContainer(store, adapter);
      addTearDown(c.dispose);
      await c.read(draftQueueProvider('u1').future);
      await c.read(draftQueueProvider('u1').notifier).sync(force: true);
      expect(
        c.read(draftQueueProvider('u1')).requireValue.first.status,
        DraftStatus.sent,
      );
      expect(
        adapter.requests.where(
          (r) => r.method == 'POST' && r.path == '/observations',
        ),
        isEmpty,
      );
      expect(
        adapter.requests.where((r) => r.path.endsWith('/media')).length,
        1,
      );
    },
  );
  test(
    'another account cannot send owner drafts and failed disk writes are not reported saved',
    () async {
      final store = MemoryDraftStore();
      store.records['another'] = jsonEncode([
        sample('other-client').copy(status: DraftStatus.queued).toJson(),
      ]);
      final adapter = emptyApi()..on('GET /auth/me', {'data': userJson()});
      final c = await queueContainer(store, adapter);
      addTearDown(c.dispose);
      await c.read(draftQueueProvider('another').future);
      await c.read(draftQueueProvider('another').notifier).sync(force: true);
      expect(adapter.requests.where((r) => r.method == 'POST'), isEmpty);
      await c.read(draftQueueProvider('u1').future);
      store.failWrite = true;
      await expectLater(
        c.read(draftQueueProvider('u1').notifier).put(sample('unsaved')),
        throwsStateError,
      );
      expect(c.read(draftQueueProvider('u1')).requireValue, isEmpty);
    },
  );
  test(
    'native store atomically persists photos and isolates owner files',
    () async {
      final root = await Directory.systemTemp.createTemp('eco-drafts-');
      addTearDown(() => root.delete(recursive: true));
      final store = FileDraftStore(directory: root);
      await store.write('owner-a', '[{"photo":"saved"}]');
      await store.write('owner-b', '[]');
      await store.write('owner-a', '[{"photo":"updated"}]');
      expect(
        await FileDraftStore(directory: root).read('owner-a'),
        '[{"photo":"updated"}]',
      );
      expect(await store.read('owner-b'), '[]');
      await expectLater(store.read('../escape'), throwsArgumentError);
    },
  );
}
