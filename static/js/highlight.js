/* Maybe expand to include Capcodes or even tripcodes */
var HighlightID = function() {
    posterID = this.querySelector(".id").textContent;
    if (posterID !== "HiddenID") {
    Array.from(document.querySelectorAll('.highlight')).forEach(
        (el) => el.classList.remove('highlight')
      );
    ps = document.getElementsByClassName(this.className)
    Array.from(ps).forEach(function(element) {
        //TODO: do this better
	element.classList.add("highlight") // Highlights thread ID: also
        if (element.parentElement.parentElement.classList.contains("post")) {
            element.parentElement.parentElement.classList.add("highlight");
        }
      });
    }
}

var CountID = function() {
    var posterID, count;
    posterID = this.textContent;
    if (posterID !== "HiddenID") {
    count = document.getElementsByClassName("id_" + posterID).length;
    this.title = count + " post" + (count === 1 ? '' : 's') + " by this ID";
    }
}

Array.from(document.getElementsByClassName("posteruid")).forEach(function(element) {
    element.addEventListener('click', HighlightID);
    element.querySelector(".id").addEventListener('mouseover', CountID);
  });
