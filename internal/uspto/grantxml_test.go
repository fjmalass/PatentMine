package uspto

import (
	"strings"
	"testing"
)

func TestFormatClaimText_Hierarchical(t *testing.T) {
	xmlInput := `
<claim-text>
21. A non-transitory computer-readable storage medium, the non-transitory computer-readable storage medium storing computer instructions to be implemented on at least one computing device including at least one processor, the computer instructions when executed by the at least one processor cause the at least one computing device to perform a method for securing an algorithm, the method comprising:
<claim-text>receiving ciphertext input data, wherein the ciphertext input data is based on plaintext input data that is encrypted using an input encryption key;</claim-text>
<claim-text>
executing obfuscated and encrypted program instructions using the ciphertext input data and at least one of a first obfuscation key and a second obfuscation key, wherein:
<claim-text>the obfuscated and encrypted program instructions are configured for concealing initial program instructions, wherein all of the obfuscated and encrypted program instructions appear identical in the program and coefficients are fixed; and</claim-text>
<claim-text>
the initial program instructions are configured for:
<claim-text>receiving the plaintext input data;</claim-text>
<claim-text>providing plaintext output data based on the algorithm;</claim-text>
</claim-text>
</claim-text>
<claim-text>and</claim-text>
<claim-text>providing ciphertext output data, wherein the ciphertext output data is configured for decryption to provide the plaintext output data.</claim-text>
</claim-text>
`

	expectedLines := []string{
		"21. A non-transitory computer-readable storage medium, the non-transitory computer-readable storage medium storing computer instructions to be implemented on at least one computing device including at least one processor, the computer instructions when executed by the at least one processor cause the at least one computing device to perform a method for securing an algorithm, the method comprising:",
		"    receiving ciphertext input data, wherein the ciphertext input data is based on plaintext input data that is encrypted using an input encryption key;",
		"    executing obfuscated and encrypted program instructions using the ciphertext input data and at least one of a first obfuscation key and a second obfuscation key, wherein:",
		"        the obfuscated and encrypted program instructions are configured for concealing initial program instructions, wherein all of the obfuscated and encrypted program instructions appear identical in the program and coefficients are fixed; and",
		"        the initial program instructions are configured for:",
		"            receiving the plaintext input data;",
		"            providing plaintext output data based on the algorithm;",
		"    and",
		"    providing ciphertext output data, wherein the ciphertext output data is configured for decryption to provide the plaintext output data.",
	}

	got := FormatClaimText(xmlInput)
	gotLines := strings.Split(got, "\n")

	if len(gotLines) != len(expectedLines) {
		t.Fatalf("expected %d lines, got %d. Result:\n%s", len(expectedLines), len(gotLines), got)
	}

	for i, expected := range expectedLines {
		if gotLines[i] != expected {
			t.Errorf("line %d mismatch:\nwant: %q\ngot : %q", i+1, expected, gotLines[i])
		}
	}
}

func TestFormatClaimText_InlineElements(t *testing.T) {
	xmlInput := `
<claim-text>
1. A system, comprising:
<claim-text>a processor; and</claim-text>
<claim-text>a memory storing <b>instructions</b> configured to execute a <claim-ref idref="CLM-00001">claim 1</claim-ref> function.</claim-text>
</claim-text>
`

	expectedLines := []string{
		"1. A system, comprising:",
		"    a processor; and",
		"    a memory storing instructions configured to execute a claim 1 function.",
	}

	got := FormatClaimText(xmlInput)
	gotLines := strings.Split(got, "\n")

	if len(gotLines) != len(expectedLines) {
		t.Fatalf("expected %d lines, got %d. Result:\n%s", len(expectedLines), len(gotLines), got)
	}

	for i, expected := range expectedLines {
		if gotLines[i] != expected {
			t.Errorf("line %d mismatch:\nwant: %q\ngot : %q", i+1, expected, gotLines[i])
		}
	}
}

func TestFormatClaimText_FallbackMalformed(t *testing.T) {
	// If the XML is malformed, it should fallback to stripXMLTags behavior.
	xmlInput := `<claim-text>1. Malformed <unclosed> tag sequence`
	expected := "1. Malformed tag sequence"
	got := FormatClaimText(xmlInput)
	if got != expected {
		t.Errorf("expected fallback text %q, got %q", expected, got)
	}
}

func TestFormatDescriptionText(t *testing.T) {
	xmlInput := `
<description id="description">
<?BRFSUM description="Brief Summary" end="lead"?>
<heading id="h-0001" level="1">TECHNICAL FIELD</heading>
<p id="p-0002" num="0001">The present disclosure relates to encryption and cryptography. More specifically, the disclosure applies to systems and methods used to obfuscate and encrypt data, programs, and algorithms.</p>
<heading id="h-0002" level="1">BACKGROUND</heading>
<p id="p-0003" num="0002">An encrypted obfuscation scheme has six components: (1) a key generation algorithm, (2) a data encryption algorithm, and (3) a verification algorithm.</p>
</description>
`

	expected := "TECHNICAL FIELD\n\n" +
		"[0001] The present disclosure relates to encryption and cryptography. More specifically, the disclosure applies to systems and methods used to obfuscate and encrypt data, programs, and algorithms.\n\n" +
		"BACKGROUND\n\n" +
		"[0002] An encrypted obfuscation scheme has six components: (1) a key generation algorithm, (2) a data encryption algorithm, and (3) a verification algorithm."

	got := FormatDescriptionText(xmlInput)
	if got != expected {
		t.Errorf("FormatDescriptionText mismatch:\nwant:\n%q\ngot:\n%q", expected, got)
	}
}

func TestFormatDescriptionText_DetailedListsAndFormulas(t *testing.T) {
	xmlInput := `
<description id="description">
<heading id="h-0005" level="1">DETAILED DESCRIPTION</heading>
<p id="p-0062" num="0061">
The disclosed obfuscation process uses encrypted ciphertext as input and is composed of a sequence of basic operations. Each of the sequence of operations may perform, but is not limited to, one or more of the following actions:
<ul id="ul0001" list-style="none">
<li id="ul0001-0001" num="0000">
<ul id="ul0002" list-style="none">
<li id="ul0002-0001" num="0062">(a) augment the input ciphertext, e.g., insert constant element, insert nonlinear transformation of each element, insert random numbers, etc.</li>
<li id="ul0002-0002" num="0063">(b) apply a linear or nonlinear operation on the input ciphertext, e.g., matrix/tensor multiply, table lookup, etc.</li>
</ul>
</li>
</ul>
</p>
<p id="p-0064" num="0067">
Each operation is further obfuscated by pre- and post-processing of the individual operators.
<br/>
<?in-line-formulae description="In-line Formulae" end="lead"?>
ƒ(<i>x</i>)=<i>XM</i><sub>0</sub>.
<?in-line-formulae description="In-line Formulae" end="tail"?>
</p>
</description>
`

	expected := "DETAILED DESCRIPTION\n\n" +
		"[0061] The disclosed obfuscation process uses encrypted ciphertext as input and is composed of a sequence of basic operations. Each of the sequence of operations may perform, but is not limited to, one or more of the following actions:\n" +
		"        (a) augment the input ciphertext, e.g., insert constant element, insert nonlinear transformation of each element, insert random numbers, etc.\n" +
		"        (b) apply a linear or nonlinear operation on the input ciphertext, e.g., matrix/tensor multiply, table lookup, etc.\n\n" +
		"[0067] Each operation is further obfuscated by pre- and post-processing of the individual operators.\n" +
		"ƒ(x)=XM0."

	got := FormatDescriptionText(xmlInput)
	if got != expected {
		t.Errorf("FormatDescriptionText mismatch:\nwant:\n%q\ngot:\n%q", expected, got)
	}
}

func TestFormatDescriptionText_EntitiesAndPermissive(t *testing.T) {
	// A fragment with special HTML entities and unescaped ampersand or unmatched tags
	// which causes strict xml.Decoder to fail and fall back to permissiveFormatDescription.
	xmlInput := `
<heading>SECTION &sect; 1.0</heading>
<p num="0001">This is a description with a degree &deg; symbol, an em-dash &#x2014; reference, and an unclosed <b>bold element.</p>
`
	expected := "SECTION § 1.0\n\n" +
		"[0001] This is a description with a degree ° symbol, an em-dash — reference, and an unclosed bold element."

	got := FormatDescriptionText(xmlInput)
	if got != expected {
		t.Errorf("Entities and permissive fallback mismatch:\nwant:\n%q\ngot:\n%q", expected, got)
	}
}

func TestFormatDescriptionText_UserSnippet(t *testing.T) {
	xmlInput := `
<heading id="h-0005" level="1">DETAILED DESCRIPTION</heading>
<p id="p-0049" num="0048">The present disclosure relates to encryption and cryptography. More specifically, the disclosure applies to systems and methods used to obfuscate and encrypt data, programs, and algorithms. Disclosed herein is a computer implemented method for providing an encrypted and obfuscated algorithm.</p>
<p id="p-0050" num="0049">The following description and figures are illustrative and are not to be construed as limiting. Numerous specific details are described to provide a thorough understanding of the disclosure. However, in certain instances, well-known or conventional details are not described in order to avoid obscuring the description. References to “one embodiment” or “an embodiment” in the present disclosure can be, but not necessarily are, references to the same embodiment and such references mean at least one of the embodiments.</p>
<p id="p-0051" num="0050">Reference in this specification to “one embodiment” or “an embodiment” means that a particular feature, structure, or characteristic described in connection with the embodiment is included in at least one embodiment of the disclosure. The appearances of the phrase “in one embodiment” in various places in the specification are not necessarily all referring to the same embodiment, nor are separate or alternative embodiments mutually exclusive of other embodiments. Moreover, various features are described which may be exhibited by some embodiments and not by others. Similarly, various requirements are described which may be requirements for some embodiments but not for other embodiments.</p>
<p id="p-0052" num="0051">The terms used in this specification generally have their ordinary meanings in the art, within the context of the disclosure, and in the specific context where each term is used. Certain terms that are used to describe the disclosure are discussed below, or elsewhere in the specification, to provide additional guidance to the practitioner regarding the description of the disclosure. For convenience, certain terms may be highlighted, for example using italics and/or quotation marks. The use of highlighting has no influence on the scope and meaning of a term; the scope and meaning of a term is the same, in the same context, whether or not it is highlighted. It will be appreciated that same thing can be said in more than one way.</p>
<p id="p-0053" num="0052">Consequently, alternative language and synonyms may be used for any one or more of the terms discussed herein, nor is any special significance to be placed upon whether or not a term is elaborated or discussed herein. Synonyms for certain terms are provided. A recital of one or more synonyms does not exclude the use of other synonyms. The use of examples anywhere in this specification, including examples of any terms discussed herein, is illustrative only, and is not intended to further limit the scope and meaning of the disclosure or of any exemplified term. Likewise, the disclosure is not limited to various embodiments given in this specification.</p>
<p id="p-0054" num="0053">Without intent to limit the scope of the disclosure, examples of instruments, apparatus, methods and their related results according to the embodiments of the present disclosure are given below. Note that titles or subtitles may be used in the examples for convenience of a reader, which in no way should limit the scope of the disclosure. Unless otherwise defined, all technical and scientific terms used herein have the same meaning as commonly understood by one of ordinary skill in the art to which this disclosure pertains. In the case of conflict, the present document, including definitions, will control.</p>
<p id="p-0055" num="0054">Unencrypted data may be referred to as plaintext data and encrypted data may be referred to as ciphertext data. The terms plaintext data and ciphertext data do not imply characters only but applies to all types of data regardless of format. The underlying information may include but is not limited to bits, quantum bits, bytes, real numbers, complex numbers, quaternions, characters, elements on cyclic groups such as Z_q, Z/Z_q, or vectors, matrices, tensors of any of the previously mentioned data types.</p>
<p id="p-0056" num="0055">Disclosed herein are methods, systems, and devices for encrypted obfuscation wherein an adversary is prevented from retrieving any information related to the inputs, outputs and/or types of computation. This disclosure describes obfuscation for ciphertexts, wherein the input data and output data are both encrypted. Unlike secure multiparty computation, obfuscation does not assume that a part of the computation is hidden from an adversary. By using encrypted input and output data all the computation can be exposed to the adversary without revealing the computation algorithms.</p>
<p id="p-0057" num="0056">When using the Turing representation, algorithms are broken into instructions that are compositions of operations between variables such as multiplication, addition, subtraction, and branching operations such as conditional if statements, recursions as well as iterations, jumps to other sets of instructions and output of the result. The embodiments of the current disclosure focus on the Turing representation of algorithms. However, the disclosure is applicable to all forms of implementation of algorithms such as and not limited to circuits or random access machines (RAMs) algorithms.</p>
<p id="p-0058" num="0057">With this disclosure, both the encrypted data and obfuscated algorithm remain secure throughout processing and evaluation. Because neither the algorithm nor the data need to be decrypted to operate, no decryption key is needed until the encrypted final results are decrypted. During operations, additional synthetic data is added to the original inputs, ensuring that the encrypted form of the input data always appears differently no matter how many times the same data is encrypted. This approach to data encryption provides resiliency to man in the middle, rainbow, or lunchtime attacks.</p>
<p id="p-0059" num="0058">Additionally, algorithms are protected by the obfuscation of the underlying operations and subsequent encryption of the coefficients in the algorithm. These obfuscated base operations can be used to process data in parallel. The encrypted coefficients and obfuscated algorithm rely on a method that enables computation directly on the encrypted input data without any decryption. During computation, no information about the data is leaked to any third party, including the entity performing the computation.</p>
<p id="p-0060" num="0059">The disclosed program obfuscation converts complex programs into a set of basic operations populated by the algorithm coefficients. Additional values are added to this set of operations by combination and augmentation, generating a tensor representation that appears completely random.</p>
<p id="p-0061" num="0060">The disclosed encryption process takes plaintext data and uses a set of operations to fully encrypt the plaintext into ciphertext. The process first augments the data with random values, then passes the augmented data through one or more operations to generate fully encrypted ciphertext that is indistinguishable from completely random values.</p>
<p id="p-0062" num="0061">
The disclosed obfuscation process uses encrypted ciphertext as input and is composed of a sequence of basic operations. Each of the sequence of operations may perform, but is not limited to, one or more of the following actions:
<ul id="ul0001" list-style="none">
<li id="ul0001-0001" num="0000">
<ul id="ul0002" list-style="none">
<li id="ul0002-0001" num="0062">(a) augment the input ciphertext, e.g., insert constant element, insert nonlinear transformation of each element, insert random numbers, etc.</li>
<li id="ul0002-0002" num="0063">(b) apply a linear or nonlinear operation on the input ciphertext, e.g., matrix/tensor multiply, table lookup, etc.</li>
<li id="ul0002-0003" num="0064">(c) apply a conditional operation to generate a single or a plurality of ciphertext output depending of the result of the conditional operation,</li>
<li id="ul0002-0004" num="0065">(d) apply a verification step such as signature creation.</li>
</ul>
</li>
</ul>
</p>
<p id="p-0063" num="0066">Each of the operations that make up the full obfuscated algorithm operates on multidimensional vectors or tensors in a similar manner to single instruction multiple data (SIMD) that are commonly implemented on computer and graphic processing units.</p>
<p id="p-0064" num="0067">
Each operation is further obfuscated by pre- and post-processing of the individual operators that compose the unit. Both the pre- and post-process methods use an oblivious random operation in such a way that the preprocess of the following step is cancelled by the post process of the prior step. The pre- and post-processing operations are fully integrated into the obfuscated algorithm as though they were part of the original obfuscation method operating on the original ciphertext. For the original function f(x), where the observation x has been converted into the vectorized form X, the initial obfuscation step is shown in equation 1.
<br/>
<?in-line-formulae description="In-line Formulae" end="lead"?>
ƒ(
<i>x</i>
)=
<i>XM</i>
<sub>0</sub>
<i>M</i>
<sub>0</sub>
<i>M</i>
<sub>1</sub>
<i>M</i>
<sub>2 </sub>
. . . .  [1]
<?in-line-formulae description="In-line Formulae" end="tail"?>
</p>
<p id="p-0065" num="0068">
The following initial encryption of the obfuscated function uses both the pre- and post-processing operations, so that the encrypted function ƒ
<sub>E </sub>
(x) is shown in equation 2.
<br/>
<?in-line-formulae description="In-line Formulae" end="lead"?>
ƒ
<sub>E</sub>
(
<i>x</i>
)=(
<i>XE</i>
<sub>0</sub>
)(
<i>E</i>
<sub>0</sub>
<sup>−1</sup>
<i>M</i>
<sub>0</sub>
<i>E</i>
<sub>1</sub>
)(
<i>E</i>
<sub>1</sub>
<sup>−1</sup>
<i>M</i>
<sub>1</sub>
<i>E</i>
<sub>2</sub>
)(
<i>E</i>
<sub>2</sub>
<sup>−1</sup>
<i>M</i>
<sub>2</sub>
<i>E</i>
<sub>3</sub>
)  [2]
<?in-line-formulae description="In-line Formulae" end="tail"?>
</p>
<p id="p-0066" num="0069">
Where E
<sub>0</sub>
, E
<sub>1</sub>
, E
<sub>2</sub>
, E
<sub>3</sub>
, . . . are encryption operators. When the encrypted algorithm is updated with new pre- and post-processing keys, then the updated encrypted function ƒ
<sub>Ê </sub>
(x) is shown in equation 3.
<br/>
<?in-line-formulae description="In-line Formulae" end="lead"?>
ƒ
<sub>Ê</sub>
(
<i>x</i>
)((
<i>XE</i>
<sub>0</sub>
)
<i>E</i>
<sub>00</sub>
)(
<i>E</i>
<sub>1</sub>
<sup>−1</sup>
(
<i>E</i>
<sub>0</sub>
<sup>−1</sup>
<i>M</i>
<sub>0</sub>
<i>E</i>
<sub>1</sub>
)
<i>E</i>
<sub>11</sub>
)(
<i>E</i>
<sub>11</sub>
<sup>−1</sup>
(
<i>E</i>
<sub>1</sub>
<sup>−1</sup>
<i>M</i>
<sub>1</sub>
<i>E</i>
<sub>2</sub>
)
<i>E</i>
<sub>22</sub>
(
<i>E</i>
<sub>22</sub>
<sup>−1</sup>
<i>E</i>
<sub>2</sub>
<sup>−1</sup>
<i>M</i>
<sub>2</sub>
<i>E</i>
<sub>3</sub>
)
<i>E</i>
<sub>33</sub>
)  [3]
<?in-line-formulae description="In-line Formulae" end="tail"?>
</p>
<p id="p-0067" num="0070">
Where E
<sub>00</sub>
, E
<sub>11</sub>
, E
<sub>22</sub>
, E
<sub>33</sub>
, . . . are new encryption operators. The encryption operators are tensors that may consist of, but are not limited to, integer, rational, or complex values, of specific construction including invertible, upper or lower triangular, diagonal, block composition, or block diagonal composition.
</p>
<p id="p-0068" num="0071">
For a given set of data and an algorithm applied to the data, the progression of encryption, obfuscation, and evaluation can be as follows:
<ul id="ul0003" list-style="none">
<li id="ul0003-0001" num="0000">
<ul id="ul0004" list-style="none">
<li id="ul0004-0001" num="0072">1. Vectorized plaintext is augmented.</li>
<li id="ul0004-0002" num="0073">2. Ciphertext is encrypted using operator dependent on the provided key.</li>
<li id="ul0004-0003" num="0074">3. Algorithm is obfuscated by transforming into sequence.</li>
<li id="ul0004-0004" num="0075">4. Sequence of linear matrices that make up algorithm is augmented.</li>
<li id="ul0004-0005" num="0076">
5. Algorithm is obfuscated using from
<b>1</b>
to N operators, the initial operator being dependent on the provided key.
</li>
<li id="ul0004-0006" num="0077">6. Ciphertext is read into initial operation of obfuscated algorithm using vector-matrix multiply.</li>
<li id="ul0004-0007" num="0078">7. Output of initial sequence of obfuscated algorithm is read into the next operation using vector-matrix multiply.</li>
<li id="ul0004-0008" num="0079">8. Repeat until the end of the obfuscated algorithm is reached.</li>
<li id="ul0004-0009" num="0080">9. Return encrypted results.</li>
<li id="ul0004-0010" num="0081">10. Decrypt results using the decryption operator dependent on the provided key.</li>
</ul>
</li>
</ul>
</p>
<p id="p-0069" num="0082">
Depending on the level of expected or required protection against attacks, the pre- and post-processing for each individual operations can be done every time the method is used, updated periodically, or left as they were initially created. The period may be a fixed time period or based on the number of computations performed between updates. The pre- and post-processing can be applied recursively, that is they can be performed multiple times on operation steps, even if they were already processed. (See DRAWINGS FIG. 3B)
</p>
`
	got := FormatDescriptionText(xmlInput)
	t.Logf("FORMATTED OUTPUT:\n%s", got)
}

func TestClassificationCodeMatchesGoogleFormat(t *testing.T) {
	// IPCR and CPC Code() must produce the same compact form that
	// googleClassifications + cleanClassification emit (no spaces).
	// Values taken directly from the user's <classifications-ipcr> example.

	// Build from the exact component values in the user's <classification-ipcr> example
	ipcr1 := USPTOGrantIPCR{Section: "G", Class: "06", Subclass: "F", MainGroup: "21", Subgroup: "14", SymbolPosition: "F"}
	if got := ipcr1.Code(); got != "G06F21/14" {
		t.Errorf("IPCR G06F21/14 = %q", got)
	}

	ipcr2 := USPTOGrantIPCR{Section: "G", Class: "06", Subclass: "F", MainGroup: "21", Subgroup: "75", SymbolPosition: "L"}
	if got := ipcr2.Code(); got != "G06F21/75" {
		t.Errorf("IPCR G06F21/75 = %q", got)
	}

	ipcr3 := USPTOGrantIPCR{Section: "H", Class: "04", Subclass: "L", MainGroup: "9", Subgroup: "00", SymbolPosition: "L"}
	if got := ipcr3.Code(); got != "H04L9/00" {
		t.Errorf("IPCR H04L9/00 = %q", got)
	}

	cpc := USPTOGrantCPC{Section: "G", Class: "06", Subclass: "F", MainGroup: "9", Subgroup: "30"}
	if got := cpc.Code(); got != "G06F9/30" {
		t.Errorf("CPC G06F9/30 = %q", got)
	}
}

