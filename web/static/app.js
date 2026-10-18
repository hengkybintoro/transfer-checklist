// Peningkatan opsional: seluruh fitur tetap berfungsi tanpa JavaScript.
(function () {
  'use strict';

  // Pencarian nama di tabel publik (FR-08).
  var search = document.getElementById('search');
  var table = document.getElementById('entry-table');
  if (search && table) {
    var rows = Array.prototype.slice.call(table.tBodies[0].rows);
    var empty = document.getElementById('no-match');
    search.addEventListener('input', function () {
      var q = search.value.trim().toLowerCase();
      var shown = 0;
      rows.forEach(function (row) {
        var match = q === '' || row.cells[0].textContent.toLowerCase().indexOf(q) !== -1;
        row.hidden = !match;
        if (match) shown++;
      });
      if (empty) empty.hidden = shown !== 0;
    });
  }

  // Konfirmasi sebelum aksi berbahaya (mis. hapus entri).
  document.addEventListener('submit', function (e) {
    var msg = e.target.getAttribute && e.target.getAttribute('data-confirm');
    if (msg && !window.confirm(msg)) e.preventDefault();
  });
})();
