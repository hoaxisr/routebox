<script lang="ts">
	import { t } from 'svelte-i18n';
	import ConfigJsonEditor from '$lib/components/config/ConfigJsonEditor.svelte';
</script>

<svelte:head>
	<title>{$t('jsonEditor.title')} - RouteBox</title>
</svelte:head>

<div class="h-full">
	<ConfigJsonEditor />
</div>
