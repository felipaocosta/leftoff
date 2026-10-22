<script lang="ts">
  // Activity indicator. With a label it announces itself to screen readers
  // (role=status); without one it is decorative (e.g. inside a busy button).
  let { label = '', size = 18 }: { label?: string; size?: number } = $props();
</script>

{#if label}
  <span class="wrap" role="status">
    <span class="spinner" style:width="{size}px" style:height="{size}px" aria-hidden="true"></span>
    <span>{label}</span>
  </span>
{:else}
  <span class="spinner" style:width="{size}px" style:height="{size}px" aria-hidden="true"></span>
{/if}

<style>
  .wrap {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--text-muted);
  }
  .spinner {
    display: inline-block;
    flex: none;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.75s linear infinite;
    vertical-align: middle;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spinner {
      animation-duration: 2.5s;
    }
  }
</style>
