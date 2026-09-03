<script setup>
// AlphaBanner — shown on every page above the nav.
// Replaces the README's "⚠️ ALPHA" block. The state is ambient
// rather than a footnote a reader can miss.
</script>

<template>
  <div class="alpha-banner" role="status" aria-live="polite">
    <span class="alpha-banner__badge">ALPHA</span>
    <span class="alpha-banner__text">
      Go-Anansi is pre-1.0. The API is unstable; expect breaking changes between
      releases. Not recommended for production yet —
      <a href="https://github.com/asaidimu/go-anansi/issues" target="_blank" rel="noopener">feedback welcome</a>.
    </span>
  </div>
</template>

<style scoped>
.alpha-banner {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.5rem 1.25rem;
  background: linear-gradient(90deg, #2b1d0a 0%, #3d2b10 100%);
  color: #f5d896;
  font-size: 0.8125rem;
  line-height: 1.4;
  border-bottom: 1px solid rgba(184, 134, 11, 0.4);
  text-align: center;
  justify-content: center;
  flex-wrap: wrap;
}

.alpha-banner__badge {
  display: inline-block;
  padding: 0.1rem 0.5rem;
  border: 1px solid #b8860b;
  border-radius: 3px;
  font-weight: 700;
  letter-spacing: 0.08em;
  font-size: 0.6875rem;
  color: #ffc04c;
  background: rgba(184, 134, 11, 0.15);
}

.alpha-banner__text a {
  color: #ffc04c;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.alpha-banner__text a:hover {
  color: #ffd680;
}
</style>
