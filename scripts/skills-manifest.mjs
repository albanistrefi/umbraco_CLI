export const EXPECTED_SKILL_COUNT = 67;

export const EXCLUDED_SKILLS = new Set([
  'umbraco-add-extension-reference',
  'umbraco-backoffice',
  'umbraco-example-generator',
  'umbraco-quickstart',
  'umbraco-review-checks',
  'umbraco-validation-checks',
]);

const foundation = [
  'umbraco-context-api',
  'umbraco-repository-pattern',
  'umbraco-extension-registry',
  'umbraco-conditions',
  'umbraco-state-management',
  'umbraco-localization',
  'umbraco-routing',
  'umbraco-notifications',
  'umbraco-umbraco-element',
  'umbraco-controllers',
];

const propertyEditors = [
  'umbraco-property-editor-ui',
  'umbraco-property-editor-schema',
  'umbraco-property-action',
  'umbraco-property-value-preset',
  'umbraco-file-upload-preview',
  'umbraco-block-editor-custom-view',
];

const richText = [
  'umbraco-tiptap-extension',
  'umbraco-tiptap-toolbar-extension',
  'umbraco-tiptap-statusbar-extension',
  'umbraco-monaco-markdown-editor-action',
];

const backend = [
  'umbraco-openapi-client',
  'umbraco-auth-provider',
  'umbraco-mfa-login-provider',
  'umbraco-granular-user-permissions',
];

const testing = [
  'umbraco-testing',
  'umbraco-unit-testing',
  'umbraco-mocked-backoffice',
  'umbraco-e2e-testing',
  'umbraco-playwright-testhelpers',
  'umbraco-test-builders',
  'umbraco-msw-testing',
  'umbraco-manifest-picker',
];

const explicitCategoryMap = new Map();
for (const name of foundation) explicitCategoryMap.set(name, 'foundation');
for (const name of propertyEditors) explicitCategoryMap.set(name, 'property-editors');
for (const name of richText) explicitCategoryMap.set(name, 'rich-text');
for (const name of backend) explicitCategoryMap.set(name, 'backend');
for (const name of testing) explicitCategoryMap.set(name, 'testing');

export function resolveCategory(skillName) {
  return explicitCategoryMap.get(skillName) ?? 'extensions';
}
