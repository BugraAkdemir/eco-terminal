import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:image_picker/image_picker.dart';
import 'package:uuid/uuid.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../../maps/data/map_services.dart';
import '../data/draft_queue.dart';

class ObservationWizardPage extends ConsumerStatefulWidget {
  const ObservationWizardPage({super.key, this.draftId});
  final String? draftId;
  @override
  ConsumerState<ObservationWizardPage> createState() => _WizardState();
}

class _WizardState extends ConsumerState<ObservationWizardPage> {
  late final String owner, id;
  int step = 0;
  Object? loadError;
  bool ready = false, busy = false, consent = false;
  Timer? autosave;
  late FieldDraft draft;
  final title = TextEditingController(),
      description = TextEditingController(),
      lat = TextEditingController(),
      lon = TextEditingController(),
      species = TextEditingController(),
      count = TextEditingController(),
      date = TextEditingController();
  @override
  void initState() {
    super.initState();
    owner = ref.read(sessionProvider).user!.id;
    id = widget.draftId ?? const Uuid().v4();
    draft = FieldDraft(
      id: id,
      body: {
        'record_type': 'observation',
        'category': 'animal',
        'observed_at': DateTime.now().toUtc().toIso8601String(),
        'location_source': 'manual',
      },
    );
    Future.microtask(_load);
  }

  Future<void> _load() async {
    try {
      final items = await ref.read(draftQueueProvider(owner).future);
      final found = items.where((d) => d.id == id).firstOrNull;
      if (found != null) {
        draft = found.status == DraftStatus.failed && found.serverId == null
            ? found.copy(status: DraftStatus.draft)
            : found;
      }
      title.text = draft.body['title'] as String? ?? '';
      description.text = draft.body['description'] as String? ?? '';
      species.text = draft.body['species_guess'] as String? ?? '';
      count.text = draft.body['count_observed']?.toString() ?? '';
      lat.text = draft.body['lat']?.toString() ?? '';
      lon.text = draft.body['lon']?.toString() ?? '';
      date.text = draft.body['observed_at'] as String;
      step = (draft.body['_step'] as int? ?? 0).clamp(0, 5);
      if (mounted) setState(() => ready = true);
    } catch (e) {
      if (mounted) setState(() => loadError = e);
    }
  }

  FieldDraft _current() => draft.copy(
    body: {
      ...draft.body,
      'title': title.text.trim(),
      'description': description.text.trim(),
      'lat': double.tryParse(lat.text),
      'lon': double.tryParse(lon.text),
      'observed_at': date.text,
      'species_guess': species.text.trim(),
      'count_observed': int.tryParse(count.text),
      '_step': step,
    },
  );
  void _changed([String? _]) {
    autosave?.cancel();
    autosave = Timer(
      const Duration(milliseconds: 350),
      () => unawaited(_save()),
    );
  }

  Future<bool> _save() async {
    try {
      draft = _current();
      await ref.read(draftQueueProvider(owner).notifier).put(draft);
      return true;
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
      return false;
    }
  }

  @override
  void dispose() {
    autosave?.cancel();
    for (final c in [title, description, lat, lon, species, count, date]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _photo(ImageSource source) async {
    if (draft.photos.length >= 3) return;
    try {
      final file = await ImagePicker().pickImage(
        source: source,
        maxWidth: 2000,
        maxHeight: 2000,
        imageQuality: 85,
      );
      if (file == null) return;
      final bytes = await file.readAsBytes();
      if (bytes.length > 4 * 1024 * 1024) {
        throw const FormatException('Photo exceeds 4 MB');
      }
      if (!mounted) return;
      setState(
        () => draft = draft.copy(
          photos: [...draft.photos, DraftPhoto(file.name, base64Encode(bytes))],
        ),
      );
      await _save();
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  Future<void> _gps() async {
    setState(() => busy = true);
    final result = await ref.read(locationServiceProvider).current();
    if (!mounted) return;
    setState(() => busy = false);
    if (result.position == null) {
      showErrorSnack(
        context,
        context.s.pick(
          'Konum alınamadı. Koordinatları elle girebilirsiniz.',
          'Location unavailable. Enter coordinates manually.',
        ),
      );
      return;
    }
    lat.text = result.position!.latitude.toStringAsFixed(6);
    lon.text = result.position!.longitude.toStringAsFixed(6);
    draft = draft.copy(body: {...draft.body, 'location_source': 'gps'});
    await _save();
    setState(() {});
  }

  bool _valid() {
    final a = double.tryParse(lat.text),
        b = double.tryParse(lon.text),
        time = DateTime.tryParse(date.text);
    final n = count.text.isEmpty ? null : int.tryParse(count.text);
    return title.text.trim().isNotEmpty &&
        title.text.length <= 200 &&
        description.text.length <= 5000 &&
        a != null &&
        a.isFinite &&
        a >= -90 &&
        a <= 90 &&
        b != null &&
        b.isFinite &&
        b >= -180 &&
        b <= 180 &&
        time != null &&
        !time.isAfter(DateTime.now().add(const Duration(minutes: 5))) &&
        time.isAfter(DateTime.now().subtract(const Duration(days: 3650))) &&
        (count.text.isEmpty || (n != null && n > 0 && n <= 1000000));
  }

  Future<void> _send() async {
    if (!_valid() || !consent) {
      showErrorSnack(
        context,
        context.s.pick(
          'Başlık, geçerli konum/tarih ve konum paylaşım onayı gerekli.',
          'A title, valid location/date and location consent are required.',
        ),
      );
      return;
    }
    setState(() => busy = true);
    autosave?.cancel();
    try {
      await ref.read(draftQueueProvider(owner).notifier).enqueue(_current());
      if (mounted) context.go('/field-queue');
    } catch (e) {
      if (mounted) {
        showErrorSnack(context, e);
        setState(() => busy = false);
      }
    }
  }

  Widget _field(
    TextEditingController controller,
    String label, {
    int lines = 1,
    TextInputType? keyboard,
    int? max,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: TextField(
      controller: controller,
      onChanged: _changed,
      maxLines: lines,
      maxLength: max,
      keyboardType: keyboard,
      decoration: InputDecoration(labelText: label),
    ),
  );
  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final names = [
      s.pick('Ne gördünüz?', 'What did you see?'),
      s.pick('Fotoğraf ekleyin', 'Add photos'),
      s.pick('Konumu belirleyin', 'Choose the location'),
      s.pick('Ne zaman, ne oldu?', 'When and what happened?'),
      s.pick('Sizin tahmininiz', 'Your guess'),
      s.pick('Kontrol edin ve gönderin', 'Review and send'),
    ];
    if (loadError != null) {
      return ErrorState(
        error: loadError!,
        onRetry: () {
          setState(() => loadError = null);
          _load();
        },
      );
    }
    if (!ready) return const Center(child: CircularProgressIndicator());
    if (draft.status != DraftStatus.draft) {
      return Center(
        child: TextButton(
          onPressed: () => context.go('/field-queue'),
          child: Text(s.pick('Kayıt kuyruğunu aç', 'Open submission queue')),
        ),
      );
    }
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  IconButton(
                    tooltip: s.pick(
                      'Taslağı sakla ve çık',
                      'Save draft and leave',
                    ),
                    onPressed: () async {
                      final saved = await _save();
                      if (saved && context.mounted) context.go('/field-queue');
                    },
                    icon: const Icon(Icons.close),
                  ),
                  Expanded(
                    child: Text(
                      names[step],
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                  ),
                  Text('${step + 1} / 6'),
                ],
              ),
            ),
            LinearProgressIndicator(value: (step + 1) / 6),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    if (step == 0) ...[
                      Text(
                        s.pick(
                          'Bir gözlem veya çevre sorunu bildirin.',
                          'Report a sighting or an environmental issue.',
                        ),
                      ),
                      const SizedBox(height: 16),
                      Wrap(
                        spacing: 12,
                        runSpacing: 12,
                        children: RecordType.values
                            .map(
                              (type) => ChoiceChip(
                                label: Text(
                                  s.pick(
                                    type == RecordType.observation
                                        ? 'Gözlem'
                                        : 'Çevre sorunu',
                                    type == RecordType.observation
                                        ? 'Sighting'
                                        : 'Environmental issue',
                                  ),
                                ),
                                selected:
                                    draft.body['record_type'] == type.wire,
                                onSelected: (_) {
                                  setState(
                                    () => draft = draft.copy(
                                      body: {
                                        ...draft.body,
                                        'record_type': type.wire,
                                      },
                                    ),
                                  );
                                  _changed();
                                },
                              ),
                            )
                            .toList(),
                      ),
                      const SizedBox(height: 24),
                      Wrap(
                        spacing: 12,
                        runSpacing: 12,
                        children: ObservationCategory.values
                            .map(
                              (cat) => ChoiceChip(
                                avatar: Icon(categoryIcon(cat)),
                                label: Padding(
                                  padding: const EdgeInsets.symmetric(
                                    vertical: 12,
                                  ),
                                  child: ConstrainedBox(
                                    constraints: const BoxConstraints(
                                      maxWidth: 180,
                                    ),
                                    child: Text(s.categoryLabel(cat)),
                                  ),
                                ),
                                selected: draft.body['category'] == cat.wire,
                                onSelected: (_) {
                                  setState(
                                    () => draft = draft.copy(
                                      body: {
                                        ...draft.body,
                                        'category': cat.wire,
                                      },
                                    ),
                                  );
                                  _changed();
                                },
                              ),
                            )
                            .toList(),
                      ),
                    ],
                    if (step == 1) ...[
                      Text(
                        s.pick(
                          'En fazla 3 fotoğraf. Bu adımı atlayabilirsiniz.',
                          'Up to 3 photos. You can skip this step.',
                        ),
                      ),
                      const SizedBox(height: 16),
                      Wrap(
                        spacing: 12,
                        runSpacing: 12,
                        children: [
                          for (final photo in draft.photos)
                            SizedBox(
                              width: 160,
                              child: Column(
                                children: [
                                  Image.memory(
                                    photo.bytes,
                                    height: 120,
                                    width: 160,
                                    fit: BoxFit.cover,
                                  ),
                                  TextButton(
                                    onPressed: () {
                                      setState(
                                        () => draft = draft.copy(
                                          photos: draft.photos
                                              .where((p) => p != photo)
                                              .toList(),
                                        ),
                                      );
                                      _changed();
                                    },
                                    child: Text(s.delete),
                                  ),
                                ],
                              ),
                            ),
                        ],
                      ),
                      Wrap(
                        spacing: 12,
                        children: [
                          OutlinedButton.icon(
                            onPressed: draft.photos.length < 3
                                ? () => _photo(ImageSource.camera)
                                : null,
                            icon: const Icon(Icons.camera_alt_outlined),
                            label: Text(s.pick('Fotoğraf çek', 'Take a photo')),
                          ),
                          OutlinedButton.icon(
                            onPressed: draft.photos.length < 3
                                ? () => _photo(ImageSource.gallery)
                                : null,
                            icon: const Icon(Icons.photo_library_outlined),
                            label: Text(
                              s.pick('Galeriden seç', 'Choose from gallery'),
                            ),
                          ),
                        ],
                      ),
                      if (draft.body['category'] == 'injured_wildlife')
                        const EmergencyNotice(),
                    ],
                    if (step == 2) ...[
                      OutlinedButton.icon(
                        onPressed: busy ? null : _gps,
                        icon: const Icon(Icons.my_location),
                        label: Text(s.pick('Konumu kullan', 'Use my location')),
                      ),
                      const SizedBox(height: 16),
                      _field(
                        lat,
                        s.pick('Enlem', 'Latitude'),
                        keyboard: const TextInputType.numberWithOptions(
                          decimal: true,
                          signed: true,
                        ),
                      ),
                      _field(
                        lon,
                        s.pick('Boylam', 'Longitude'),
                        keyboard: const TextInputType.numberWithOptions(
                          decimal: true,
                          signed: true,
                        ),
                      ),
                      Text(
                        s.pick(
                          'GPS izni vermezseniz konumu elle yazabilirsiniz. Kastamonu sınırı sunucuda kontrol edilir.',
                          'You can enter coordinates manually without GPS permission. The server checks the Kastamonu boundary.',
                        ),
                      ),
                    ],
                    if (step == 3) ...[
                      _field(
                        title,
                        s.pick('Kısa başlık', 'Short title'),
                        max: 200,
                      ),
                      _field(
                        description,
                        s.pick('Gördüğünüzü anlatın', 'Describe what you saw'),
                        lines: 4,
                        max: 5000,
                      ),
                      _field(
                        date,
                        s.pick(
                          'Gözlem zamanı (ISO 8601, UTC)',
                          'Observation time (ISO 8601, UTC)',
                        ),
                      ),
                      TextButton.icon(
                        onPressed: () async {
                          final d = await showDatePicker(
                            context: context,
                            initialDate:
                                DateTime.tryParse(date.text)?.toLocal() ??
                                DateTime.now(),
                            firstDate: DateTime.now().subtract(
                              const Duration(days: 3649),
                            ),
                            lastDate: DateTime.now(),
                          );
                          if (d == null || !context.mounted) return;
                          final t = await showTimePicker(
                            context: context,
                            initialTime: TimeOfDay.fromDateTime(
                              DateTime.tryParse(date.text)?.toLocal() ??
                                  DateTime.now(),
                            ),
                          );
                          if (t == null) return;
                          date.text = DateTime(
                            d.year,
                            d.month,
                            d.day,
                            t.hour,
                            t.minute,
                          ).toUtc().toIso8601String();
                          _changed();
                        },
                        icon: const Icon(Icons.calendar_month),
                        label: Text(
                          s.pick('Tarih ve saat seç', 'Choose date and time'),
                        ),
                      ),
                    ],
                    if (step == 4) ...[
                      _field(
                        species,
                        s.pick(
                          'Tür tahmininiz (isteğe bağlı)',
                          'Species guess (optional)',
                        ),
                        max: 200,
                      ),
                      _field(
                        count,
                        s.pick(
                          'Gördüğünüz adet (isteğe bağlı)',
                          'Number seen (optional)',
                        ),
                        keyboard: TextInputType.number,
                      ),
                      Text(
                        s.pick(
                          'Bilmiyorsanız boş bırakın. Türü uzman doğrulayacak.',
                          'Leave blank if unsure. An expert will verify the species.',
                        ),
                      ),
                    ],
                    if (step == 5) ...[
                      Text(
                        title.text.isEmpty
                            ? s.pick('Başlık eksik', 'Missing title')
                            : title.text,
                        style: Theme.of(context).textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 16),
                      Text(description.text),
                      const SizedBox(height: 16),
                      Text(
                        '${lat.text}, ${lon.text}\n${date.text}\n${draft.photos.length} ${s.pick('fotoğraf', 'photos')}',
                      ),
                      CheckboxListTile(
                        contentPadding: EdgeInsets.zero,
                        value: consent,
                        onChanged: (v) => setState(() => consent = v ?? false),
                        title: Text(
                          s.pick(
                            'Konumumu bu gözlemle paylaşmayı kabul ediyorum.',
                            'I agree to share my location with this observation.',
                          ),
                        ),
                        subtitle: Text(
                          s.pick(
                            'Hassas türlerin tam konumu yalnızca sahip ve uzmanlara gösterilir.',
                            'Exact locations of sensitive species are shown only to owners and reviewers.',
                          ),
                        ),
                      ),
                      Text(
                        s.pick(
                          'Önce bu cihaza kaydedilir. Bağlantı gelince uygulama açıkken otomatik gönderilir; uzman onayı bekler.',
                          'Saved to this device first. Syncs automatically while the app is open when connected, then awaits expert review.',
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  if (step > 0)
                    OutlinedButton(
                      onPressed: busy
                          ? null
                          : () {
                              setState(() => step--);
                              _changed();
                            },
                      child: Text(s.pick('Geri', 'Back')),
                    ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: FilledButton(
                      key: const ValueKey('wizard-next'),
                      onPressed: busy
                          ? null
                          : step == 5
                          ? _send
                          : () {
                              setState(() => step++);
                              _changed();
                            },
                      child: Text(
                        step == 5
                            ? s.pick('Kaydet ve gönder', 'Save and send')
                            : s.continueLabel,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
