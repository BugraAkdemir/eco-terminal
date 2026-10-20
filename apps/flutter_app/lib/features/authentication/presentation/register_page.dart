import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/router/route_access.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/theme/tokens.dart';
import '../data/validators.dart';
import 'auth_scaffold.dart';

class RegisterPage extends ConsumerStatefulWidget {
  const RegisterPage({super.key, this.from});
  final String? from;

  @override
  ConsumerState<RegisterPage> createState() => _RegisterPageState();
}

class _RegisterPageState extends ConsumerState<RegisterPage> {
  final _formKey = GlobalKey<FormState>();
  final _name = TextEditingController();
  final _email = TextEditingController();
  final _password = TextEditingController();
  bool _busy = false;
  bool _obscure = true;
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final s = context.s;
    if (!(_formKey.currentState?.validate() ?? false)) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(sessionProvider.notifier)
          .register(_email.text, _password.text, displayName: _name.text);
      if (!mounted) return;
      ScaffoldMessenger.maybeOf(context)
          ?.showSnackBar(SnackBar(content: Text(s.registerSuccess)));
      context.go(widget.from ?? AppPaths.home);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = s.errorText(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    return AuthScaffold(
      title: s.register,
      child: Form(
        key: _formKey,
        child: AutofillGroup(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (_error != null) FormErrorBanner(message: _error!),
              TextFormField(
                key: const ValueKey('register-name'),
                controller: _name,
                textInputAction: TextInputAction.next,
                autofillHints: const [AutofillHints.name],
                decoration: InputDecoration(labelText: s.displayName),
              ),
              const SizedBox(height: Dimens.md),
              TextFormField(
                key: const ValueKey('register-email'),
                controller: _email,
                keyboardType: TextInputType.emailAddress,
                autofillHints: const [AutofillHints.email],
                textInputAction: TextInputAction.next,
                decoration: InputDecoration(labelText: s.email),
                validator: (v) => AuthValidators.email(s, v),
              ),
              const SizedBox(height: Dimens.md),
              TextFormField(
                key: const ValueKey('register-password'),
                controller: _password,
                obscureText: _obscure,
                autofillHints: const [AutofillHints.newPassword],
                textInputAction: TextInputAction.done,
                onFieldSubmitted: (_) => _submit(),
                decoration: InputDecoration(
                  labelText: s.password,
                  helperText: s.passwordTooShort,
                  suffixIcon: IconButton(
                    tooltip: s.password,
                    icon: Icon(_obscure ? Icons.visibility : Icons.visibility_off),
                    onPressed: () => setState(() => _obscure = !_obscure),
                  ),
                ),
                validator: (v) => AuthValidators.password(s, v),
              ),
              const SizedBox(height: Dimens.xl),
              FilledButton(
                key: const ValueKey('register-submit'),
                onPressed: _busy ? null : _submit,
                child: _busy
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(s.register),
              ),
              const SizedBox(height: Dimens.md),
              Wrap(
                alignment: WrapAlignment.center,
                crossAxisAlignment: WrapCrossAlignment.center,
                children: [
                  Text(s.haveAccount),
                  TextButton(
                    key: const ValueKey('go-login'),
                    onPressed: () => context.go(
                      Uri(
                        path: AppPaths.login,
                        queryParameters:
                            widget.from == null ? null : {'from': widget.from},
                      ).toString(),
                    ),
                    child: Text(s.login),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
