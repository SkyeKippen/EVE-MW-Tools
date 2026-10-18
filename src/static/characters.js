// My Characters: the de-authorize prompts. The first asks to confirm and
// offers to delete the data; ticking that leads to a second prompt, where
// the data is only deleted once "I understand ... irreversible" is ticked too.
(function () {
    const form = document.getElementById("deauthorize-form");
    const confirmDialog = document.getElementById("deauthorize-confirm");
    const deleteDialog = document.getElementById("deauthorize-delete");
    const deleteData = document.getElementById("deauthorize-delete-data");
    const confirmDelete = document.getElementById("deauthorize-confirm-delete");

    function submit(withData) {
        form.elements["delete-data"].value = withData ? "1" : "";
        form.elements["confirm-delete"].value = withData ? "1" : "";
        form.submit();
    }

    document.querySelectorAll("[data-deauthorize]").forEach(function (button) {
        button.addEventListener("click", function () {
            form.action = button.dataset.deauthorize;
            form.elements["character-id"].value = button.dataset.characterId || "";
            document.querySelectorAll(".deauthorize-dialog [data-target]").forEach(function (el) {
                el.textContent = button.dataset.target;
            });
            deleteData.checked = false;
            confirmDelete.checked = false;
            confirmDialog.showModal();
        });
    });

    document.getElementById("deauthorize-confirm-button").addEventListener("click", function () {
        if (!deleteData.checked) {
            submit(false);
            return;
        }
        confirmDialog.close();
        deleteDialog.showModal();
    });

    document.getElementById("deauthorize-delete-button").addEventListener("click", function () {
        submit(confirmDelete.checked);
    });

    document.querySelectorAll("[data-close-dialog]").forEach(function (button) {
        button.addEventListener("click", function () {
            button.closest("dialog").close();
        });
    });
})();
